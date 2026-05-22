package email

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MailTMProvider uses the free mail.tm API for temporary email addresses.
// Uses real domains (wshu.net) that are less likely to be flagged by AWS TES checks.
type MailTMProvider struct {
	apiURL   string
	address  string
	password string
	jwt      string
	client   *http.Client
}

type mailTMAccount struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type mailTMToken struct {
	Token string `json:"token"`
	ID    string `json:"id"`
}

type mailTMMessage struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	From    struct {
		Address string `json:"address"`
	} `json:"from"`
}

type mailTMMessages struct {
	Member []mailTMMessage `json:"hydra:member"`
}

type mailTMMessageContent struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
	Intro   string `json:"intro"`
}

func NewMailTMProvider(proxy string) *MailTMProvider {
	tr := &http.Transport{}
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &MailTMProvider{
		apiURL: "https://api.mail.tm",
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tr,
		},
	}
}

func (m *MailTMProvider) Create() string {
	domain, err := m.getDomain()
	if err != nil {
		log.Printf("[MailTM] 获取域名失败: %v", err)
		return ""
	}
	log.Printf("[MailTM] 使用域名: %s", domain)

	ts := time.Now().UnixMilli()
	m.address = fmt.Sprintf("t%d@%s", ts, domain)
	m.password = "AutoReg_" + fmt.Sprintf("%d", ts)
	log.Printf("[MailTM] 创建邮箱: %s", m.address)

	if err := m.createAccount(); err != nil {
		log.Printf("[MailTM] 创建账户失败: %v", err)
		return ""
	}

	time.Sleep(2 * time.Second)
	if err := m.getToken(); err != nil {
		log.Printf("[MailTM] 获取 Token 失败: %v", err)
		return ""
	}

	log.Printf("[MailTM] 创建成功: %s", m.address)
	return m.address
}

func (m *MailTMProvider) GetAddress() string {
	return m.address
}

func (m *MailTMProvider) WaitForCode(timeout, interval int) (string, error) {
	maxRetries := timeout / interval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		code, err := m.fetchCode()
		if err != nil {
			if attempt%6 == 0 {
				log.Printf("[MailTM] [%d/%d] 请求异常: %v", attempt, maxRetries, err)
			}
		} else if code != "" {
			return code, nil
		}
		if attempt%6 == 0 {
			log.Printf("[MailTM] [%d/%d] 等待邮件...", attempt, maxRetries)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

func (m *MailTMProvider) getDomain() (string, error) {
	req, err := http.NewRequest("GET", m.apiURL+"/domains", nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var domains []struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal(body, &domains); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("没有可用域名")
	}
	return domains[0].Domain, nil
}

func (m *MailTMProvider) createAccount() error {
	payload := map[string]string{
		"address":  m.address,
		"password": m.password,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", m.apiURL+"/accounts", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 201 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var account mailTMAccount
	if err := json.Unmarshal(respBody, &account); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}
	if account.Address != "" {
		m.address = account.Address
	}
	return nil
}

func (m *MailTMProvider) getToken() error {
	payload := map[string]string{
		"address":  m.address,
		"password": m.password,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", m.apiURL+"/token", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var token mailTMToken
	if err := json.Unmarshal(respBody, &token); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}
	if token.Token == "" {
		return fmt.Errorf("Token 为空")
	}
	m.jwt = token.Token
	return nil
}

func (m *MailTMProvider) fetchCode() (string, error) {
	req, err := http.NewRequest("GET", m.apiURL+"/messages", nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.jwt)
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var msgs mailTMMessages
	if err := json.Unmarshal(body, &msgs); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if len(msgs.Member) == 0 {
		return "", nil
	}

	return m.fetchMessageContent(msgs.Member[0].ID)
}

func (m *MailTMProvider) fetchMessageContent(msgID string) (string, error) {
	req, err := http.NewRequest("GET", m.apiURL+"/messages/"+msgID, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.jwt)
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var content mailTMMessageContent
	if err := json.Unmarshal(body, &content); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	text := content.Subject + " " + content.Text + " " + content.Intro
	if code := ExtractCode(text); code != "" {
		return code, nil
	}

	if code := ExtractCode(content.HTML); code != "" {
		return code, nil
	}

	return "", nil
}
