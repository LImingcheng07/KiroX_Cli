package email

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	httputil "reg_go/internal/http"
)

// OutlookAccount Outlook 邮箱账号
type OutlookAccount struct {
	Email        string
	Password     string
	ClientID     string
	RefreshToken string
}

// ParseOutlookCSV 解析 outlook.csv
func ParseOutlookCSV(path string) ([]OutlookAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var accounts []OutlookAccount
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(strings.TrimSpace(normalized), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "----", 4)
		if len(parts) != 4 {
			log.Printf("跳过格式错误的行: %s", line[:min(50, len(line))])
			continue
		}
		accounts = append(accounts, OutlookAccount{
			Email:        parts[0],
			Password:     parts[1],
			ClientID:     parts[2],
			RefreshToken: parts[3],
		})
	}
	return accounts, nil
}

// ParseOutlookLines 从文本内容直接解析 Outlook 账号 (Web UI 使用)
// 支持两种格式:
// 1. 换行分隔: 每行一个账号
// 2. 空格分隔: 账号之间用空格隔开
func ParseOutlookLines(data string) []OutlookAccount {
	var accounts []OutlookAccount
	data = strings.TrimSpace(data)
	if data == "" {
		return accounts
	}

	// 先尝试按换行分割
	lines := strings.Split(data, "\n")
	
	// 如果只有一行，可能是空格分隔的格式
	if len(lines) == 1 {
		// 尝试按空格分割（账号格式: email----password----clientid----token）
		// 每个账号以空格结尾，下一个账号开始
		parts := strings.Fields(data) // Fields 会按空白字符分割并去除空白
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			fields := strings.SplitN(part, "----", 4)
			if len(fields) == 4 {
				accounts = append(accounts, OutlookAccount{
					Email:        strings.TrimSpace(fields[0]),
					Password:     strings.TrimSpace(fields[1]),
					ClientID:     strings.TrimSpace(fields[2]),
					RefreshToken: strings.TrimSpace(fields[3]),
				})
			}
		}
	} else {
		// 多行格式，按行解析
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "----", 4)
			if len(parts) == 4 {
				accounts = append(accounts, OutlookAccount{
					Email:        strings.TrimSpace(parts[0]),
					Password:     strings.TrimSpace(parts[1]),
					ClientID:     strings.TrimSpace(parts[2]),
					RefreshToken: strings.TrimSpace(parts[3]),
				})
			}
		}
	}
	
	return accounts
}

// RefreshOutlookToken 用 refresh_token 获取 access_token
func RefreshOutlookToken(acc OutlookAccount, proxy, chromeVer string) (string, error) {
	form := url.Values{
		"client_id":     {acc.ClientID},
		"refresh_token": {acc.RefreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {"https://outlook.office.com/IMAP.AccessAsUser.All offline_access"},
	}

	client := httputil.NewTLSClient(proxy, true, chromeVer)
	req, err := http.NewRequest("POST",
		"https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("刷新失败 %d: %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("响应中无 access_token")
	}
	return token, nil
}

// buildXOAuth2 构建 XOAUTH2 认证字符串
func buildXOAuth2(email, accessToken string) string {
	auth := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", email, accessToken)
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// imapClient 简易 IMAP 客户端
type imapClient struct {
	conn   net.Conn
	reader *bufio.Reader
	tag    int
}

// newIMAPClient 连接 Outlook IMAP
func newIMAPClient() (*imapClient, error) {
	tlsConfig := &tls.Config{ServerName: "outlook.office365.com"}
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 15 * time.Second},
		"tcp", "outlook.office365.com:993", tlsConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %v", err)
	}

	c := &imapClient{conn: conn, reader: bufio.NewReader(conn), tag: 0}
	greeting, err := c.readLine()
	if err != nil {
		conn.Close()
		return nil, err
	}
	log.Printf("[IMAP] %s", greeting)
	return c, nil
}

func (c *imapClient) sendCommand(cmd string) (string, error) {
	c.tag++
	tagStr := fmt.Sprintf("A%03d", c.tag)
	line := fmt.Sprintf("%s %s\r\n", tagStr, cmd)
	_, err := c.conn.Write([]byte(line))
	if err != nil {
		return "", err
	}
	return tagStr, nil
}

func (c *imapClient) readLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (c *imapClient) readUntilTag(tag string) ([]string, string, error) {
	var lines []string
	for {
		line, err := c.readLine()
		if err != nil {
			return lines, "", err
		}
		if strings.HasPrefix(line, tag+" ") {
			return lines, line, nil
		}
		lines = append(lines, line)
	}
}

func (c *imapClient) authenticate(email, accessToken string) error {
	xoauth2 := buildXOAuth2(email, accessToken)
	tag, err := c.sendCommand("AUTHENTICATE XOAUTH2 " + xoauth2)
	if err != nil {
		return err
	}
	_, result, err := c.readUntilTag(tag)
	if err != nil {
		return err
	}
	if !strings.Contains(result, "OK") {
		return fmt.Errorf("认证失败: %s", result)
	}
	log.Println("[IMAP] 认证成功")

	// Exchange 服务器认证后需要短暂等待才能操作邮箱
	// exchangelabs.com 后端需要更长时间完成状态迁移
	time.Sleep(2 * time.Second)

	// 认证后发一次 CAPABILITY 强制状态迁移，绕过 Exchange "Invalid state" 问题
	capTag, err := c.sendCommand("CAPABILITY")
	if err != nil {
		return err
	}
	if _, _, err := c.readUntilTag(capTag); err != nil {
		return err
	}
	return nil
}

func (c *imapClient) selectInbox() (int, error) {
	for retry := 0; retry < 5; retry++ {
		tag, err := c.sendCommand(`SELECT "INBOX"`)
		if err != nil {
			return 0, err
		}
		lines, result, err := c.readUntilTag(tag)
		if err != nil {
			return 0, err
		}
		if strings.Contains(result, "OK") {
			total := 0
			for _, line := range lines {
				if strings.Contains(line, "EXISTS") {
					fmt.Sscanf(line, "* %d EXISTS", &total)
				}
			}
			return total, nil
		}
		if retry < 4 {
			log.Printf("[IMAP] SELECT INBOX 失败 (%s), 重试 %d/5...", result, retry+1)
			// 失败时重新发送 CAPABILITY 触发状态迁移
			capTag, _ := c.sendCommand("CAPABILITY")
			c.readUntilTag(capTag)
			// 重试间隔递增: 1s, 2s, 3s, 4s
			time.Sleep(time.Duration(1+retry) * time.Second)
		} else {
			return 0, fmt.Errorf("SELECT 失败: %s", result)
		}
	}
	return 0, fmt.Errorf("SELECT INBOX 重试耗尽")
}

func (c *imapClient) close() {
	c.sendCommand("LOGOUT")
	c.conn.Close()
}

// fetchLatestBody 获取指定邮件的正文并解码
func (c *imapClient) fetchLatestBody(seq int) (string, error) {
	if seq <= 0 {
		return "", fmt.Errorf("无效的邮件序号")
	}
	tag, err := c.sendCommand(fmt.Sprintf("FETCH %d (BODY.PEEK[TEXT])", seq))
	if err != nil {
		return "", err
	}
	lines, result, err := c.readUntilTag(tag)
	if err != nil {
		return "", err
	}
	if !strings.Contains(result, "OK") {
		return "", fmt.Errorf("FETCH TEXT 失败: %s", result)
	}

	var rawLines []string
	inBody := false
	for _, line := range lines {
		if strings.Contains(line, "FETCH") {
			inBody = true
			continue
		}
		if line == ")" {
			continue
		}
		if inBody {
			rawLines = append(rawLines, line)
		}
	}

	raw := strings.Join(rawLines, "\n")

	// 尝试解码 MIME base64 内容
	parts := strings.Split(raw, "------=_Part_")
	var decoded string
	for _, part := range parts {
		if strings.Contains(part, "base64") {
			idx := strings.Index(part, "base64")
			content := part[idx+6:]
			b64 := strings.Map(func(r rune) rune {
				if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
					return -1
				}
				return r
			}, content)
			if data, err := base64.StdEncoding.DecodeString(b64); err == nil {
				decoded += string(data) + " "
			}
		}
	}
	if decoded != "" {
		return decoded, nil
	}

	// 整体 base64 解码
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	if data, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return string(data), nil
	}

	return raw, nil
}

// fetchLatestHeaders 获取最新 N 封邮件的头部
func (c *imapClient) fetchLatestHeaders(total, count int) error {
	if total == 0 {
		log.Println("[IMAP] 收件箱为空")
		return nil
	}
	start := total - count + 1
	if start < 1 {
		start = 1
	}

	tag, err := c.sendCommand(fmt.Sprintf("FETCH %d:%d (BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE)])", start, total))
	if err != nil {
		return err
	}
	lines, result, err := c.readUntilTag(tag)
	if err != nil {
		return err
	}
	if !strings.Contains(result, "OK") {
		return fmt.Errorf("FETCH 失败: %s", result)
	}

	var buf bytes.Buffer
	idx := 0
	for _, line := range lines {
		if strings.Contains(line, "FETCH") {
			if buf.Len() > 0 {
				fmt.Printf("--- 邮件 %d ---\n%s\n", idx, buf.String())
			}
			buf.Reset()
			idx++
		} else if line != ")" && line != "" {
			buf.WriteString(line + "\n")
		}
	}
	if buf.Len() > 0 {
		fmt.Printf("--- 邮件 %d ---\n%s\n", idx, buf.String())
	}
	return nil
}

// WaitForOTP 通过 IMAP 轮询等待 AWS 验证码
func WaitForOTP(acc OutlookAccount, beforeCount, timeout, interval int, proxy, chromeVer string) (string, error) {
	log.Printf("[Outlook IMAP] 等待验证码, 邮箱=%s, 发送前邮件数=%d", acc.Email, beforeCount)

	accessToken, err := RefreshOutlookToken(acc, proxy, chromeVer)
	if err != nil {
		return "", fmt.Errorf("刷新 Outlook Token 失败: %v", err)
	}

	maxRetries := timeout / interval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		client, err := newIMAPClient()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[Outlook IMAP] 连接失败: %v, 重试中...", err)
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		if err := client.authenticate(acc.Email, accessToken); err != nil {
			client.close()
			accessToken, _ = RefreshOutlookToken(acc, proxy, chromeVer)
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		total, err := client.selectInbox()
		if err != nil {
			log.Printf("[Outlook IMAP] [%d/%d] SELECT INBOX 失败: %v", attempt, maxRetries, err)
			client.close()
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		if total <= beforeCount {
			client.close()
			if attempt%5 == 0 {
				log.Printf("[Outlook IMAP] [%d/%d] 暂无新邮件 (当前%d封)...", attempt, maxRetries, total)
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		for i := total; i > beforeCount; i-- {
			body, err := client.fetchLatestBody(i)
			if err != nil {
				continue
			}
			code := ExtractCode(body)
			if code != "" {
				log.Printf("[Outlook IMAP] 获取到验证码: %s", code)
				client.close()
				return code, nil
			}
		}

		client.close()
		if attempt%5 == 0 {
			log.Printf("[Outlook IMAP] [%d/%d] 新邮件中未找到验证码...", attempt, maxRetries)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

// GetInboxCount 获取收件箱当前邮件数量
func GetInboxCount(acc OutlookAccount, proxy, chromeVer string) (int, error) {
	accessToken, err := RefreshOutlookToken(acc, proxy, chromeVer)
	if err != nil {
		return 0, fmt.Errorf("刷新 Outlook Token 失败: %v", err)
	}
	client, err := newIMAPClient()
	if err != nil {
		return 0, fmt.Errorf("连接 IMAP 失败: %v", err)
	}
	defer client.close()
	if err := client.authenticate(acc.Email, accessToken); err != nil {
		return 0, fmt.Errorf("IMAP 认证失败: %v", err)
	}
	total, err := client.selectInbox()
	if err != nil {
		return 0, fmt.Errorf("选择收件箱失败: %v", err)
	}
	return total, nil
}

// RunIMAPTest IMAP 测试入口
func RunIMAPTest(csvPath string, index int) {
	accounts, err := ParseOutlookCSV(csvPath)
	if err != nil {
		log.Fatalf("读取 CSV 失败: %v", err)
	}
	if len(accounts) == 0 {
		log.Fatal("CSV 中没有账号")
	}

	if index < 0 || index >= len(accounts) {
		index = 0
	}
	acc := accounts[index]
	log.Printf("测试邮箱: %s (第 %d 个)", acc.Email, index+1)

	log.Println("刷新 OAuth Token...")
	accessToken, err := RefreshOutlookToken(acc, "", "144.0.0.0")
	if err != nil {
		log.Fatalf("刷新 Token 失败: %v", err)
	}
	log.Printf("Token 获取成功 (长度: %d)", len(accessToken))

	log.Println("连接 IMAP 服务器...")
	client, err := newIMAPClient()
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer client.close()

	if err := client.authenticate(acc.Email, accessToken); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	total, err := client.selectInbox()
	if err != nil {
		log.Fatalf("选择收件箱失败: %v", err)
	}
	log.Printf("收件箱共 %d 封邮件", total)

	if err := client.fetchLatestHeaders(total, 5); err != nil {
		log.Fatalf("获取邮件失败: %v", err)
	}

	log.Println("IMAP 测试完成")
}
