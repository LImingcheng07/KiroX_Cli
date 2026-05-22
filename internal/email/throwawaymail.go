package email

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ThrowawayMailProvider uses the free throwawaymail.app API.
// No API key, no auth required.
type ThrowawayMailProvider struct {
	apiURL    string
	mailboxID string
	address   string
	client    *http.Client
}

type twmMailbox struct {
	MailboxID string `json:"mailbox_id"`
	Address   string `json:"address"`
}

type twmMessage struct {
	MessageID   string `json:"message_id"`
	Subject     string `json:"subject"`
	FromAddress string `json:"from_address"`
}

type twmMessageContent struct {
	MessageID   string `json:"message_id"`
	Subject     string `json:"subject"`
	FromAddress string `json:"from_address"`
	Text        string `json:"text"`
	HTML        string `json:"html"`
}

func NewThrowawayMailProvider(proxy string) *ThrowawayMailProvider {
	tr := &http.Transport{}
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &ThrowawayMailProvider{
		apiURL: "https://throwawaymail.app",
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tr,
		},
	}
}

func (t *ThrowawayMailProvider) Create() string {
	req, err := http.NewRequest("POST", t.apiURL+"/api/mailboxes", nil)
	if err != nil {
		log.Printf("[ThrowawayMail] 创建请求失败: %v", err)
		return ""
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		log.Printf("[ThrowawayMail] 请求失败: %v", err)
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		log.Printf("[ThrowawayMail] HTTP %d: %s", resp.StatusCode, string(body))
		return ""
	}

	var mb twmMailbox
	if err := json.Unmarshal(body, &mb); err != nil {
		log.Printf("[ThrowawayMail] 解析响应失败: %v", err)
		return ""
	}

	t.mailboxID = mb.MailboxID
	t.address = mb.Address
	log.Printf("[ThrowawayMail] 创建成功: %s", t.address)
	return t.address
}

func (t *ThrowawayMailProvider) GetAddress() string {
	return t.address
}

func (t *ThrowawayMailProvider) WaitForCode(timeout, interval int) (string, error) {
	maxRetries := timeout / interval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		code, err := t.fetchCode()
		if err != nil {
			if attempt%6 == 0 {
				log.Printf("[ThrowawayMail] [%d/%d] 请求异常: %v", attempt, maxRetries, err)
			}
		} else if code != "" {
			return code, nil
		}
		if attempt%6 == 0 {
			log.Printf("[ThrowawayMail] [%d/%d] 等待邮件...", attempt, maxRetries)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

func (t *ThrowawayMailProvider) fetchCode() (string, error) {
	if t.mailboxID == "" {
		return "", fmt.Errorf("mailboxID 为空")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/mailboxes/%s/messages", t.apiURL, t.mailboxID), nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", nil
	}

	var msgs []twmMessage
	if err := json.Unmarshal(body, &msgs); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	for _, msg := range msgs {
		re := regexp.MustCompile(`\b(\d{6})\b`)
		if m := re.FindStringSubmatch(msg.Subject); len(m) > 1 {
			return m[1], nil
		}

		code, err := t.fetchMessageContent(msg.MessageID)
		if err == nil && code != "" {
			return code, nil
		}
	}

	return "", nil
}

func (t *ThrowawayMailProvider) fetchMessageContent(msgID string) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/mailboxes/%s/messages/%s", t.apiURL, t.mailboxID, msgID), nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var content twmMessageContent
	if err := json.Unmarshal(body, &content); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	text := content.Subject + " " + content.Text + " " + content.HTML
	// AWS OTP format: "XXXXXX is your verification code" or just a 6-digit code
	re := regexp.MustCompile(`\b(\d{6})\b`)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return m[1], nil
	}

	return "", nil
}

func IsThrowawayMailAddress(addr string) bool {
	return strings.HasSuffix(addr, "@throwawaymail.app")
}
