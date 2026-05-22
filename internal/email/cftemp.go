package email

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/quotedprintable"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// CFProvider Cloudflare Temp Mail 临时邮箱提供者
type CFProvider struct {
	baseURL  string
	adminKey string
	customPW string
	jwt      string
	address  string
	client   *http.Client
}

func NewCFTempMailService(baseURL, adminKey, customPW, proxy string) TempEmailService {
	tr := &http.Transport{}
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &CFProvider{
		baseURL:  baseURL,
		adminKey: adminKey,
		customPW: customPW,
		client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
	}
}

func (c *CFProvider) Create() string {
	url := c.baseURL + "/admin/new_address"
	name := fmt.Sprintf("u%d", rand.Int63())
	payload := map[string]interface{}{
		"enablePrefix": true,
		"name":         name,
		"domain":       "",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[CFTemp] 创建请求失败: %v", err)
		return ""
	}
	req.Header.Set("x-admin-auth", c.adminKey)
	req.Header.Set("Content-Type", "application/json")
	if c.customPW != "" {
		req.Header.Set("x-custom-auth", c.customPW)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[CFTemp] 请求失败: %v", err)
		return ""
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Printf("[CFTemp] HTTP %d: %s", resp.StatusCode, string(respBody))
		return ""
	}

	var result struct {
		JWT     string `json:"jwt"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[CFTemp] 解析响应失败: %v", err)
		return ""
	}
	if result.Address == "" {
		log.Printf("[CFTemp] 响应中没有邮箱地址: %s", string(respBody))
		return ""
	}
	c.jwt = result.JWT
	c.address = result.Address
	log.Printf("[CFTemp] 创建成功: %s", c.address)
	return c.address
}

func (c *CFProvider) GetAddress() string {
	return c.address
}

func (c *CFProvider) WaitForCode(timeout, interval int) (string, error) {
	maxRetries := timeout / interval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		code, err := c.fetchCode()
		if err != nil {
			if attempt%6 == 0 {
				log.Printf("[CFTemp] [%d/%d] 请求异常: %v", attempt, maxRetries, err)
			}
		} else if code != "" {
			return code, nil
		}
		if attempt%6 == 0 {
			log.Printf("[CFTemp] [%d/%d] 等待邮件...", attempt, maxRetries)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

// parseRawBody 从 RFC822 raw 数据中提取可读正文文本
func parseRawBody(raw string) string {
	// 分离 header 和 body (第一个空行之后是 body)
	parts := strings.SplitN(raw, "\r\n\r\n", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(raw, "\n\n", 2)
	}
	headerBlock := parts[0]
	body := raw
	if len(parts) >= 2 {
		body = parts[1]
	}

	// 提取 subject (可能 base64 编码)
	subject := ""
	for _, line := range strings.Split(headerBlock, "\r\n") {
		if strings.HasPrefix(line, "Subject: ") ||
			strings.HasPrefix(line, "subject: ") {
			subj := line[9:]
			// 解码 =?UTF-8?B?...?= 格式
			if strings.HasPrefix(subj, "=?UTF-8?B?") && strings.HasSuffix(subj, "?=") {
				b64 := subj[10 : len(subj)-2]
				if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
					subject = string(decoded)
				}
			} else {
				subject = subj
			}
			break
		}
	}

	// 解码 quoted-printable body
	var bodyText string
	// 尝试 QP 解码
	qpReader := quotedprintable.NewReader(strings.NewReader(body))
	if decoded, err := io.ReadAll(qpReader); err == nil {
		bodyText = string(decoded)
	} else {
		bodyText = body
	}

	// 去除 HTML 标签
	htmlTag := regexp.MustCompile(`<[^>]*>`)
	plainText := htmlTag.ReplaceAllString(bodyText, " ")

	// 合并多个空白
	ws := regexp.MustCompile(`\s+`)
	plainText = ws.ReplaceAllString(plainText, " ")

	// 过滤掉 CSS 属性值中的颜色 (#xxx)
	plainText = regexp.MustCompile(`#[0-9a-fA-F]{3,6}\b`).ReplaceAllString(plainText, "")

	// 只保留可读字符
	var clean strings.Builder
	for _, r := range plainText {
		if r == ' ' || r == '\n' || unicode.IsPrint(r) {
			clean.WriteRune(r)
		}
	}

	result := subject + " " + clean.String()
	// log.Printf("[CFTemp] 解析后的正文: %s", result[:min(len(result), 200)])
	return result
}

func (c *CFProvider) fetchCode() (string, error) {
	url := fmt.Sprintf("%s/api/mails?limit=20&offset=0", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.jwt)
	if c.customPW != "" {
		req.Header.Set("x-custom-auth", c.customPW)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []struct {
			Raw string `json:"raw"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}
	if len(result.Results) == 0 {
		return "", nil
	}

	// 遍历所有邮件（最新的在前），解析 raw 数据
	for _, mail := range result.Results {
		content := parseRawBody(mail.Raw)
		if code := ExtractCode(content); code != "" {
			return code, nil
		}
	}
	return "", nil
}
