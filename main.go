package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/joho/godotenv"

	"reg_go/internal/core"
	"reg_go/internal/email"
	httputil "reg_go/internal/http"
)

func main() {
	// 加载 .env 文件
	godotenv.Load()

	count := flag.Int("n", 1, "注册数量")
	output := flag.String("o", "", "结果输出到json件")
	proxy := flag.String("p", "", "代理地址")
	delay := flag.Int("d", 3, "串行模式间隔秒数")
	concurrency := flag.Int("j", 1, "并发数")
	debug := flag.Bool("debug", false, "调试模式")
	imapTest := flag.Bool("imap", false, "IMAP 邮件测试模式")
	imapCSV := flag.String("imap-csv", "outlook.csv", "Outlook CSV 文件路径")
	imapIndex := flag.Int("imap-i", 0, "测试第几个账号 (从0开始)")
	useOutlook := flag.Bool("outlook", false, "使用 Outlook 邮箱注册")
	outlookCSV := flag.String("outlook-csv", "outlook.csv", "Outlook CSV 文件路径")
	moEmailURL := flag.String("moemail-url", getEnv("MOEMAIL_BASE_URL", "https://api.moemail.app"), "MoEmail API 地址")
	moEmailAPIKey := flag.String("moemail-key", getEnv("MOEMAIL_API_KEY", ""), "MoEmail API Key")
	useCFTemp := flag.Bool("cftemp", false, "使用 Cloudflare Temp Mail")
	useMailTM := flag.Bool("mailtm", false, "使用 MailTM 临时邮箱 (免费, 无 API Key 要求)")
	useThrowawayMail := flag.Bool("throwawaymail", false, "使用 ThrowawayMail 临时邮箱 (免费, 无 API Key 要求)")
	useGmail := flag.Bool("gmail", false, "使用 Gmail 邮箱 (需要 App Password)")
	gmailEmail := flag.String("gmail-email", "", "Gmail 邮箱地址")
	gmailAppPwd := flag.String("gmail-app-password", "", "Gmail App Password")
	cftempURL := flag.String("cftemp-url", getEnv("CFTEMP_BASE_URL", ""), "Cloudflare Temp Mail 地址")
	cftempAdminKey := flag.String("cftemp-admin-key", getEnv("CFTEMP_ADMIN_KEY", ""), "Cloudflare Temp Mail 管理密码")
	cftempCustomAuth := flag.String("cftemp-custom-auth", getEnv("CFTEMP_CUSTOM_AUTH", ""), "Cloudflare Temp Mail 站点密码")
	flag.Parse()

	if *debug {
		log.SetFlags(log.Ltime | log.Lmicroseconds)
	} else {
		log.SetFlags(log.Ltime)
	}

	outPath := *output
	if outPath == "" {
		cwd, _ := os.Getwd()
		outPath = filepath.Join(cwd, "output", "results.json")
	}

	// IMAP 邮件测试模式
	if *imapTest {
		email.RunIMAPTest(*imapCSV, *imapIndex)
		return
	}

	os.MkdirAll(filepath.Dir(outPath), 0755)

	cfg := core.NewConfig()
	cfg.Proxy = *proxy
	cfg.Debug = *debug
	cfg.MoEmailBaseURL = *moEmailURL
	cfg.MoEmailAPIKey = *moEmailAPIKey

	if *useCFTemp {
		cfg.UseCFTemp = true
		cfg.CFTempBaseURL = *cftempURL
		cfg.CFTempAdminKey = *cftempAdminKey
		cfg.CFTempCustomAuth = *cftempCustomAuth
	}
	if *useMailTM {
		cfg.UseMailTM = true
	}
	if *useThrowawayMail {
		cfg.UseThrowawayMail = true
	}
	if *useGmail {
		cfg.UseGmail = true
		if *gmailEmail != "" && *gmailAppPwd != "" {
			cfg.GmailAccount = &email.GmailAccount{Email: *gmailEmail, AppPassword: *gmailAppPwd}
		}
	}

	// 模式选择
	var modeDesc string
	var outlookAccounts []email.OutlookAccount
	switch {
	case *useOutlook:
		cfg.UseOutlook = true
		cfg.OutlookCSV = *outlookCSV
		var err error
		outlookAccounts, err = email.ParseOutlookCSV(*outlookCSV)
		if err != nil {
			log.Fatalf("读取 Outlook CSV 失败: %v", err)
		}
		if len(outlookAccounts) == 0 {
			log.Fatalf("Outlook CSV 中没有账号")
		}
		if len(outlookAccounts) < *count {
			log.Printf("注意: 需要注册 %d 个, CSV 中有 %d 个账号 (已注册的会自动跳过)", *count, len(outlookAccounts))
		}
		modeDesc = fmt.Sprintf("Outlook 模式: 已加载 %d 个账号", len(outlookAccounts))
	case *useCFTemp:
		modeDesc = "Cloudflare Temp Mail 模式"
	case *useMailTM:
		modeDesc = "MailTM 临时邮箱模式"
	case *useThrowawayMail:
		modeDesc = "ThrowawayMail 临时邮箱模式"
	case *useGmail:
		if cfg.GmailAccount == nil {
			log.Fatal("Gmail 模式需要指定 -gmail-email 和 -gmail-app-password")
		}
		modeDesc = fmt.Sprintf("Gmail 模式: %s", cfg.GmailAccount.Email)
	default:
		modeDesc = "临时邮箱模式 (MoeMail)"
	}
	log.Println(modeDesc)

	checkIPRegion(cfg.Proxy)

	runBatch(*count, cfg, outPath, *delay, *concurrency, outlookAccounts)
}

func runBatch(count int, cfg *core.Config, output string, delay, concurrency int, outlookAccounts []email.OutlookAccount) {
	// 加载已有结果，实现增量追加
	var existing []map[string]interface{}
	if data, err := os.ReadFile(output); err == nil {
		json.Unmarshal(data, &existing)
	}

	var results []map[string]interface{}
	var mu sync.Mutex

	var accountIdx int64

	getNextAccount := func() (email.OutlookAccount, int, bool) {
		idx := int(atomic.AddInt64(&accountIdx, 1) - 1)
		if idx >= len(outlookAccounts) {
			return email.OutlookAccount{}, idx, false
		}
		return outlookAccounts[idx], idx, true
	}

	doTask := func(taskNum int) {
		for {
			taskCfg := *cfg
			taskCfg.Password = core.GenPassword()

			var acc email.OutlookAccount
			if cfg.UseOutlook {
				var ok bool
				var accIdx int
				acc, accIdx, ok = getNextAccount()
				if !ok {
					log.Printf("[任务%d] Outlook 账号已用完，停止", taskNum+1)
					return
				}
				taskCfg.OutlookAccount = &acc
				log.Printf("[%d/%d] 开始注册 (账号 #%d %s)", taskNum+1, count, accIdx+1, acc.Email)
			} else {
				log.Printf("[%d/%d] 开始注册", taskNum+1, count)
			}

			reg := core.NewRegistrar(&taskCfg)
			result := reg.Run()

			errStr, _ := result["error"].(string)
			if errStr == "邮箱已注册过，跳过" {
				emailAddr, _ := result["email"].(string)
				if emailAddr == "" && cfg.UseOutlook {
					emailAddr = acc.Email
				}
				log.Printf("[%d/%d] %s 已注册，自动从文件剔除并尝试下一个账号", taskNum+1, count, emailAddr)
				if cfg.UseOutlook && cfg.OutlookCSV != "" && emailAddr != "" {
					removeAccountFromCSV(cfg.OutlookCSV, emailAddr, &mu)
				}
				continue
			}

			mu.Lock()
			results = append(results, result)
			saveResults(existing, results, output)
			okCount := 0
			for _, r := range results {
				if r["status"] == "success" {
					okCount++
				}
			}
			mu.Unlock()

			if result["status"] == "success" {
				log.Printf("[%d/%d] %s 成功 (累计 %d 成功)", len(results), count, result["email"], okCount)
				emailAddr, _ := result["email"].(string)
				if emailAddr == "" && cfg.UseOutlook {
					emailAddr = acc.Email
				}
				if cfg.UseOutlook && cfg.OutlookCSV != "" && emailAddr != "" {
					removeAccountFromCSV(cfg.OutlookCSV, emailAddr, &mu)
				}
			} else {
				if len(errStr) > 60 {
					errStr = errStr[:60]
				}
				log.Printf("[%d/%d] %s 失败: %s", len(results), count, result["email"], errStr)
			}
			return
		}
	}

	if concurrency > 1 {
		log.Printf("并发模式: %d 并发, 共 %d 个", concurrency, count)
		t0 := time.Now()
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup
		for i := 0; i < count; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(idx int) {
				defer wg.Done()
				defer func() { <-sem }()
				doTask(idx)
			}(i)
		}
		wg.Wait()
		elapsed := time.Since(t0).Seconds()
		log.Printf("耗时 %.1fs, 平均 %.1fs/号", elapsed, elapsed/float64(max(len(results), 1)))
	} else {
		for i := 0; i < count; i++ {
			doTask(i)
			if delay > 0 && i < count-1 {
				time.Sleep(time.Duration(delay) * time.Second)
			}
		}
	}

	okCount := 0
	failCount := 0
	for _, r := range results {
		switch r["status"] {
		case "success":
			okCount++
		case "failed":
			failCount++
		}
	}
	log.Printf("完成! 成功: %d, 失败: %d, 总计: %d", okCount, failCount, len(results))
	log.Printf("结果: %s", output)
}

// saveResults 保存结果到 JSON (增量追加，existing 为已有数据)
func saveResults(existing []map[string]interface{}, results []map[string]interface{}, path string) {
	// 以已有数据为基础
	outputData := make([]map[string]interface{}, len(existing))
	copy(outputData, existing)

	// 追加本次新注册的成功账号
	for _, r := range results {
		if r["status"] == "success" {
			at, _ := r["aws_token"].(map[string]interface{})
			if at == nil {
				at = map[string]interface{}{}
			}
			verify, _ := r["verify"].(map[string]interface{})
			item := map[string]interface{}{
				"refreshToken": at["refreshToken"],
				"provider":     "BuilderId",
				"clientId":     r["client_id"],
				"clientSecret": r["client_secret"],
				"region":       "us-east-1",
				"email":        r["email"],
			}
			if verify != nil {
				item["creditUsed"] = verify["credit_used"]
				item["creditLimit"] = verify["credit_limit"]
				item["subscription"] = verify["subscription"]
			}
			outputData = append(outputData, item)
		}
	}
	b, _ := json.MarshalIndent(outputData, "", "  ")
	os.WriteFile(path, b, 0644)
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// removeAccountFromCSV 从 CSV 中移除已消费/已注册的账号
func removeAccountFromCSV(csvPath, email string, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(csvPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	var newLines []string
	for _, line := range lines {
		trimLine := strings.TrimSpace(line)
		if trimLine == "" {
			continue
		}
		// 根据邮箱匹配 (以 email---- 开头)
		if !strings.HasPrefix(trimLine, email+"----") {
			newLines = append(newLines, line)
		}
	}
	os.WriteFile(csvPath, []byte(strings.Join(newLines, "\n")), 0644)
}

// geoResult holds parsed IP geo information
type geoResult struct {
	IP           string
	Country      string
	Region       string
	City         string
	ISP          string
	Organization string
}

// httpDoer matches the common Do method of both *http.Client and tls_client.HttpClient
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// fetchGeo 请求指定 URL 并尝试解析 IP 地理信息，返回解析到的结果和是否成功
func fetchGeo(client httpDoer, url string) (*geoResult, bool) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false
	}

	type result struct {
		r  *geoResult
		ok bool
	}
	ch := make(chan result, 1)

	go func() {
		resp, err := client.Do(req)
		if err != nil {
			ch <- result{nil, false}
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			ch <- result{nil, false}
			return
		}

		ip, _ := data["ip"].(string)
		if ip == "" {
			ch <- result{nil, false}
			return
		}
		r := &geoResult{IP: ip}
		r.Country, _ = data["country"].(string)
		r.Region, _ = data["region"].(string)
		r.City, _ = data["city"].(string)
		r.ISP, _ = data["isp"].(string)
		r.Organization, _ = data["organization"].(string)
		if r.ISP == "" {
			r.ISP = r.Organization
		}
		// ipinfo.io uses "org" field (e.g. "AS1234 ISP Name")
		if r.ISP == "" {
			if org, ok := data["org"].(string); ok {
				r.ISP = org
			}
		}
		ch <- result{r, true}
	}()

	select {
	case r := <-ch:
		return r.r, r.ok
	case <-time.After(5 * time.Second):
		return nil, false
	}
}

// checkIPRegion 检测当前 IP 的归属地并打印
func checkIPRegion(proxy string) {
	log.Println("正在检测当前 IP 地区...")
	client := httputil.NewNoRedirectTLSClient(proxy, "120")

	// 按优先级尝试多个 geo 服务
	services := []string{
		"https://ipinfo.io/json",
		"https://api.ip.sb/geoip",
	}
	for _, url := range services {
		if result, ok := fetchGeo(client, url); ok {
			log.Printf("当前 IP: %s [%s %s %s] ISP: %s",
				result.IP, result.Country, result.Region, result.City, result.ISP)
			return
		}
	}

	log.Printf("IP 检测异常: 所有 geo 服务均不可用，忽略")
}
