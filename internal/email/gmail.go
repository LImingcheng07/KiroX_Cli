package email

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// GmailAccount Gmail 邮箱账号 (使用 App Password)
type GmailAccount struct {
	Email       string
	AppPassword string
}

type GmailProvider struct {
	account GmailAccount
	proxy   string
}

func NewGmailProvider(email, appPassword, proxy string) *GmailProvider {
	return &GmailProvider{
		account: GmailAccount{Email: email, AppPassword: appPassword},
		proxy:   proxy,
	}
}

func (g *GmailProvider) Create() string {
	return g.account.Email
}

func (g *GmailProvider) GetAddress() string {
	return g.account.Email
}

func (g *GmailProvider) WaitForCode(timeout, interval int) (string, error) {
	return WaitForGmailOTP(g.account, timeout, interval, g.proxy)
}

// ParseGmailCSV 解析 Gmail CSV 文件 (email----apppassword)
func ParseGmailCSV(path string) ([]GmailAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var accounts []GmailAccount
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(strings.TrimSpace(normalized), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "----", 2)
		if len(parts) != 2 {
			log.Printf("跳过格式错误的行: %s", line[:min(50, len(line))])
			continue
		}
		accounts = append(accounts, GmailAccount{
			Email:       strings.TrimSpace(parts[0]),
			AppPassword: strings.TrimSpace(parts[1]),
		})
	}
	return accounts, nil
}

func (c *imapClient) gmailLogin(email, appPassword string) error {
	tag, err := c.sendCommand(fmt.Sprintf("LOGIN %s %s", email, appPassword))
	if err != nil {
		return err
	}
	_, result, err := c.readUntilTag(tag)
	if err != nil {
		return err
	}
	if !strings.Contains(result, "OK") {
		return fmt.Errorf("Gmail IMAP 登录失败: %s", result)
	}
	log.Println("[Gmail IMAP] 登录成功")
	return nil
}

// newGmailIMAPClient 连接 Gmail IMAP 服务器
func newGmailIMAPClient() (*imapClient, error) {
	tlsConfig := &tls.Config{ServerName: "imap.gmail.com"}
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 15 * time.Second},
		"tcp", "imap.gmail.com:993", tlsConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("连接 Gmail IMAP 失败: %v", err)
	}
	c := &imapClient{conn: conn, reader: bufio.NewReader(conn), tag: 0}
	greeting, err := c.readLine()
	if err != nil {
		conn.Close()
		return nil, err
	}
	log.Printf("[Gmail IMAP] %s", greeting)
	return c, nil
}

// WaitForGmailOTP 通过 Gmail IMAP 轮询等待 AWS 验证码
func WaitForGmailOTP(acc GmailAccount, timeout, interval int, proxy string) (string, error) {
	log.Printf("[Gmail IMAP] 等待验证码, 邮箱=%s", acc.Email)

	maxRetries := timeout / interval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		client, err := newGmailIMAPClient()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[Gmail IMAP] 连接失败: %v, 重试中...", err)
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		if err := client.gmailLogin(acc.Email, acc.AppPassword); err != nil {
			client.close()
			log.Printf("[Gmail IMAP] 登录失败: %v", err)
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		total, err := client.selectInbox()
		if err != nil {
			log.Printf("[Gmail IMAP] [%d/%d] SELECT INBOX 失败: %v", attempt, maxRetries, err)
			client.close()
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		if total == 0 {
			client.close()
			if attempt%5 == 0 {
				log.Printf("[Gmail IMAP] [%d/%d] 收件箱为空...", attempt, maxRetries)
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		for i := total; i >= 1; i-- {
			body, err := client.fetchLatestBody(i)
			if err != nil {
				continue
			}
			code := ExtractCode(body)
			if code != "" {
				log.Printf("[Gmail IMAP] 获取到验证码: %s", code)
				client.close()
				return code, nil
			}
		}

		client.close()
		if attempt%5 == 0 {
			log.Printf("[Gmail IMAP] [%d/%d] 邮件中未找到验证码...", attempt, maxRetries)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}
