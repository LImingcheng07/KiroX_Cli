package core

import (
	"math/rand"
	"strings"

	"reg_go/internal/email"
)

// Config 注册配置
type Config struct {
	// AWS
	OIDCBase    string
	SigninBase  string
	ProfileBase string
	ViewBase    string
	PortalBase  string
	DirectoryID string
	StartURL    string

	// Kiro
	KiroBase        string
	KiroRedirectURI string

	// 默认值
	Password string
	FullName string

	// 运行时
	Proxy string
	Debug bool

	// MoEmail 配置
	MoEmailBaseURL string
	MoEmailAPIKey  string

	// Cloudflare Temp Mail 配置
	UseCFTemp        bool
	CFTempBaseURL    string
	CFTempAdminKey   string
	CFTempCustomAuth string

	// MailTM 临时邮箱配置
	UseMailTM bool

	// ThrowawayMail 临时邮箱配置
	UseThrowawayMail bool

	// Gmail 配置
	UseGmail     bool
	GmailCSV     string
	GmailAccount *email.GmailAccount

	// Outlook 模式
	UseOutlook     bool
	OutlookCSV     string
	OutlookAccount *email.OutlookAccount // 当前使用的 Outlook 账号
}

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		OIDCBase:        "https://oidc.us-east-1.amazonaws.com",
		SigninBase:      "https://us-east-1.signin.aws",
		ProfileBase:     "https://profile.aws.amazon.com",
		ViewBase:        "https://view.awsapps.com",
		PortalBase:      "https://portal.sso.us-east-1.amazonaws.com",
		DirectoryID:     "d-9067642ac7",
		StartURL:        "https://view.awsapps.com/start",
		KiroBase:        "https://app.kiro.dev",
		KiroRedirectURI: "https://app.kiro.dev/signin/oauth",
		Password:        GenPassword(),
		FullName:        "Test User",
	}
}

// GenPassword 生成随机密码
func GenPassword() string {
	upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lower := "abcdefghijklmnopqrstuvwxyz"
	digits := "0123456789"
	special := "!@#$%^&*"

	var b strings.Builder
	for i := 0; i < 3; i++ {
		b.WriteByte(upper[rand.Intn(len(upper))])
	}
	for i := 0; i < 6; i++ {
		b.WriteByte(lower[rand.Intn(len(lower))])
	}
	for i := 0; i < 3; i++ {
		b.WriteByte(digits[rand.Intn(len(digits))])
	}
	for i := 0; i < 2; i++ {
		b.WriteByte(special[rand.Intn(len(special))])
	}
	pw := []byte(b.String())
	rand.Shuffle(len(pw), func(i, j int) { pw[i], pw[j] = pw[j], pw[i] })
	return string(pw)
}
