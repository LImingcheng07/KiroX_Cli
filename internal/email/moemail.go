package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	httputil "reg_go/internal/http"
	tls_client "github.com/bogdanfinn/tls-client"
)

// MoEmailProvider MoEmail 临时邮箱提供者
type MoEmailProvider struct {
	baseURL string
	apiKey  string
	emailID string
	address string
	client  tls_client.HttpClient
}

// NewMoEmailProvider 创建 MoEmail 邮箱实例
func NewMoEmailProvider(baseURL, apiKey, proxy, chromeVer string) *MoEmailProvider {
	provider := &MoEmailProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  httputil.NewTLSClient(proxy, true, chromeVer),
	}
	return provider
}

// GetConfig 获取系统配置
func (m *MoEmailProvider) GetConfig() ([]string, error) {
	url := m.baseURL + "/api/config"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("X-API-Key", m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		EmailDomains string `json:"emailDomains"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.EmailDomains == "" {
		return nil, fmt.Errorf("没有可用的域名")
	}

	// 分割逗号分隔的域名字符串
	domains := []string{}
	for _, d := range strings.Split(result.EmailDomains, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}

	return domains, nil
}

// Create 创建临时邮箱
func (m *MoEmailProvider) Create() string {
	// 先获取可用域名
	domains, err := m.GetConfig()
	if err != nil {
		log.Printf("[MoEmail] 获取域名列表失败: %v", err)
		return ""
	}
	if len(domains) == 0 {
		log.Printf("[MoEmail] 没有可用的域名")
		return ""
	}

	// 使用第一个可用域名
	domain := domains[0]
	log.Printf("[MoEmail] 使用域名: %s", domain)

	url := m.baseURL + "/api/emails/generate"

	payload := map[string]interface{}{
		"name":       "",
		"expiryTime": 3600000, // 1小时
		"domain":     domain,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[MoEmail] 创建请求失败: %v", err)
		return ""
	}

	req.Header.Set("X-API-Key", m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		log.Printf("[MoEmail] 请求失败: %v", err)
		return ""
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		// 解析错误信息
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			log.Printf("[MoEmail] 创建失败: %s", errResp.Error)
			// 权限不足的特殊提示
			if resp.StatusCode == 403 {
				log.Printf("[MoEmail] 提示: 需要公爵或皇帝角色才能使用 OpenAPI，请在 MoEmail 个人中心升级角色")
			}
		} else {
			log.Printf("[MoEmail] HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return ""
	}

	var result struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[MoEmail] 解析响应失败: %v, body: %s", err, string(respBody))
		return ""
	}

	if result.Email == "" {
		log.Printf("[MoEmail] 响应中没有邮箱地址, body: %s", string(respBody))
		return ""
	}

	m.emailID = result.ID
	m.address = result.Email
	log.Printf("[MoEmail] 创建成功: %s", m.address)

	return m.address
}

// GetAddress 获取邮箱地址
func (m *MoEmailProvider) GetAddress() string {
	return m.address
}

// WaitForCode 等待验证码
func (m *MoEmailProvider) WaitForCode(timeout, interval int) (string, error) {
	if m.emailID == "" {
		return "", fmt.Errorf("邮箱未创建")
	}

	maxRetries := timeout / interval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		code, err := m.fetchCode()
		if err != nil {
			if attempt%6 == 0 {
				log.Printf("[MoEmail] [%d/%d] 请求异常: %v", attempt, maxRetries, err)
			}
		} else if code != "" {
			return code, nil
		}

		if attempt%6 == 0 && err == nil {
			log.Printf("[MoEmail] [%d/%d] 等待邮件...", attempt, maxRetries)
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}

	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

// fetchCode 获取验证码
func (m *MoEmailProvider) fetchCode() (string, error) {
	url := fmt.Sprintf("%s/api/emails/%s", m.baseURL, m.emailID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("X-API-Key", m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Messages []struct {
			ID          string `json:"id"`
			FromAddress string `json:"from_address"`
			Subject     string `json:"subject"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	// 没有邮件
	if len(result.Messages) == 0 {
		return "", nil
	}

	// 获取最新邮件的详细内容
	latestMsg := result.Messages[0]
	return m.fetchMessageContent(latestMsg.ID)
}

// fetchMessageContent 获取邮件内容并提取验证码
func (m *MoEmailProvider) fetchMessageContent(messageID string) (string, error) {
	url := fmt.Sprintf("%s/api/emails/%s/%s", m.baseURL, m.emailID, messageID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("X-API-Key", m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Message struct {
			Subject string `json:"subject"`
			Content string `json:"content"`
			HTML    string `json:"html"`
		} `json:"message"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	// 从主题和内容中提取验证码
	text := result.Message.Subject + " " + result.Message.Content
	if code := ExtractCode(text); code != "" {
		return code, nil
	}

	// 如果纯文本没找到，尝试从 HTML 中提取
	if code := ExtractCode(result.Message.HTML); code != "" {
		return code, nil
	}

	return "", nil
}

// ExtractCode 提取 6 位验证码
func ExtractCode(text string) string {
	patterns := []string{
		`(?i)(?:verification code|code|验证码)[：:\s]+(\d{6})`,
		`(\d{6})\s*(?:is your|为您的)`,
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		m := re.FindStringSubmatch(text)
		if len(m) == 2 && !isBlacklisted(m[1]) {
			return m[1]
		}
	}
	re := regexp.MustCompile(`\b(\d{6})\b`)
	m := re.FindStringSubmatch(text)
	if len(m) == 2 && !isBlacklisted(m[1]) {
		return m[1]
	}
	return ""
}

func isBlacklisted(code string) bool {
	return code == "000000" || code == "111111" || code == "123456"
}
