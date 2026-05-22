package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	"reg_go/internal/browser"
	"reg_go/internal/crypto"
	"reg_go/internal/email"
	httputil "reg_go/internal/http"
)

// Registrar 完整的注册流程
type Registrar struct {
	Cfg      *Config
	Client   tls_client.HttpClient
	Cookies  map[string]string
	Identity *browser.BrowserIdentity
	FPCtx    *browser.FingerprintContext

	VisitorID        string
	Email            string
	EmailSvc         email.TempEmailService // 临时邮箱服务
	ClientID         string
	ClientSecret     string
	DeviceCode       string
	UserCode         string
	WorkflowHandle   string
	WorkflowID       string
	WorkflowState    string
	Ubid             string
	RegCode          string
	SignState        string
	AuthCode         string
	SSOState         string
	WdcCSRFToken     string
	SSOToken         string
	KiroCodeVerifier string
	KiroState        string
	KiroClientID     string
	KiroClientSecret string
	KiroRedirectPort int

	JWE *crypto.JWEEncryptor

	// Outlook 模式: 发送验证码前的邮件数量
	OutlookMailCount int
}

// NewRegistrar 创建注册器
func NewRegistrar(cfg *Config) *Registrar {
	identity := browser.RandomIdentity()
	log.Printf("[指纹] Chrome: %s | GPU: %s | 内存: %dGB | 核心: %d | 分辨率: %dx%d (%d-bit)",
		identity.ChromeVer, identity.GPUModel, identity.DeviceMemory, identity.HardwareConcurrency,
		identity.Screen.Width, identity.Screen.Height, identity.Screen.ColorDepth)

	client := httputil.NewTLSClient(cfg.Proxy, true, identity.ChromeVer)
	return &Registrar{
		Cfg:       cfg,
		Client:    client,
		Cookies:   make(map[string]string),
		Identity:  identity,
		FPCtx:     browser.NewFPContext(identity),
		VisitorID: httputil.VisitorID(),
		JWE:       &crypto.JWEEncryptor{},
	}
}

// DoPost 发送 POST 请求
func (r *Registrar) DoPost(url string, payload interface{}, headers map[string]string) ([]byte, map[string][]string, error) {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, nil, err
	}
	httputil.SetHeaders(req, headers)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.Header, nil
}

// DoGet 发送 GET 请求，返回完整信息
func (r *Registrar) DoGet(url string, headers map[string]string) ([]byte, int, map[string][]string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	httputil.SetHeaders(req, headers)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, resp.Header, nil
}

// DoPostRaw 发送 POST 请求，返回状态码
func (r *Registrar) DoPostRaw(url string, payload interface{}, headers map[string]string) ([]byte, int, map[string][]string, error) {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, 0, nil, err
	}
	httputil.SetHeaders(req, headers)
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, resp.Header, nil
}

// GenFP 生成指纹
func (r *Registrar) GenFP(pageType, eventType string, emailLen int, emailAddr string) string {
	return r.GenFPWithTime(pageType, eventType, 0, emailLen, emailAddr)
}

// GenFPWithTime 生成指纹（指定页面停留时间）
func (r *Registrar) GenFPWithTime(pageType, eventType string, timeOnPage, emailLen int, emailAddr string) string {
	did := r.Cfg.DirectoryID
	var loc, ref string

	switch pageType {
	case "signin":
		loc = fmt.Sprintf("%s/platform/%s/login?workflowStateHandle=%s", r.Cfg.SigninBase, did, r.WorkflowHandle)
	case "signup":
		loc = fmt.Sprintf("%s/platform/%s/signup?workflowStateHandle=%s", r.Cfg.SigninBase, did, r.WorkflowHandle)
	default: // profile
		if eventType == "PageSubmit" {
			loc = fmt.Sprintf("%s/?workflowID=%s#/signup/enter-email", r.Cfg.ProfileBase, r.WorkflowID)
		} else {
			loc = fmt.Sprintf("%s/?workflowID=%s#/signup/start", r.Cfg.ProfileBase, r.WorkflowID)
		}
		if r.WorkflowID == "" {
			loc = r.Cfg.ProfileBase + "/"
		}
	}

	if pageType == "profile" {
		ref = fmt.Sprintf("%s/platform/%s/signup?workflowStateHandle=%s", r.Cfg.SigninBase, did, r.WorkflowHandle)
	} else {
		ref = r.Cfg.ViewBase + "/"
	}

	return browser.GenerateFingerprint(r.Identity, loc, ref, r.FPCtx, pageType, eventType, timeOnPage, emailLen, emailAddr)
}

// Step1OIDC OIDC 注册
func (r *Registrar) Step1OIDC() error {
	log.Println("[1] OIDC 注册")
	body, _, err := r.DoPost(r.Cfg.OIDCBase+"/client/register", map[string]interface{}{
		"clientName": "Amazon Q Developer for command line",
		"clientType": "public",
		"scopes":     []string{"codewhisperer:completions", "codewhisperer:analysis", "codewhisperer:conversations", "codewhisperer:transformations", "codewhisperer:taskassist"},
	}, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	var data map[string]interface{}
	json.Unmarshal(body, &data)
	r.ClientID, _ = data["clientId"].(string)
	r.ClientSecret, _ = data["clientSecret"].(string)
	if r.ClientID == "" {
		return fmt.Errorf("OIDC 注册失败: %s", string(body))
	}
	return nil
}

// Step2Device 设备授权
func (r *Registrar) Step2Device() error {
	log.Println("[2] 设备授权")
	body, _, err := r.DoPost(r.Cfg.OIDCBase+"/device_authorization", map[string]interface{}{
		"clientId": r.ClientID, "clientSecret": r.ClientSecret,
		"startUrl": r.Cfg.StartURL,
	}, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	var data map[string]interface{}
	json.Unmarshal(body, &data)
	r.DeviceCode, _ = data["deviceCode"].(string)
	r.UserCode, _ = data["userCode"].(string)
	log.Printf("user_code=%s", r.UserCode)
	return nil
}

// Step3Email 获取邮箱 (临时邮箱、Cloudflare Temp Mail、Outlook)
func (r *Registrar) Step3Email() error {
	if r.Cfg.UseOutlook && r.Cfg.OutlookAccount != nil {
		log.Println("[3] 使用 Outlook 邮箱")
		r.Email = r.Cfg.OutlookAccount.Email
		log.Printf("email=%s", r.Email)
		return nil
	}
	if r.Cfg.UseCFTemp {
		log.Println("[3] 使用 Cloudflare Temp Mail")
		r.EmailSvc = email.NewCFTempMailService(r.Cfg.CFTempBaseURL, r.Cfg.CFTempAdminKey, r.Cfg.CFTempCustomAuth, r.Cfg.Proxy)
		r.Email = r.EmailSvc.Create()
		log.Printf("email=%s", r.Email)
		return nil
	}
	if r.Cfg.UseMailTM {
		log.Println("[3] 使用 MailTM 临时邮箱")
		r.EmailSvc = email.NewMailTMProvider(r.Cfg.Proxy)
		r.Email = r.EmailSvc.Create()
		log.Printf("email=%s", r.Email)
		return nil
	}
	if r.Cfg.UseThrowawayMail {
		log.Println("[3] 使用 ThrowawayMail 临时邮箱")
		r.EmailSvc = email.NewThrowawayMailProvider(r.Cfg.Proxy)
		r.Email = r.EmailSvc.Create()
		log.Printf("email=%s", r.Email)
		return nil
	}
	if r.Cfg.UseGmail && r.Cfg.GmailAccount != nil {
		log.Println("[3] 使用 Gmail 邮箱")
		r.Email = r.Cfg.GmailAccount.Email
		r.EmailSvc = email.NewGmailProvider(r.Cfg.GmailAccount.Email, r.Cfg.GmailAccount.AppPassword, r.Cfg.Proxy)
		log.Printf("email=%s", r.Email)
		return nil
	}
	log.Println("[3] 创建临时邮箱")
	r.EmailSvc = email.NewMoEmailService(r.Cfg.MoEmailBaseURL, r.Cfg.MoEmailAPIKey, r.Cfg.Proxy, r.Identity.ChromeVer)
	r.Email = r.EmailSvc.Create()
	log.Printf("email=%s", r.Email)
	return nil
}

// Step4Portal Portal 初始化
func (r *Registrar) Step4Portal() error {
	log.Println("[4] Portal 初始化")
	r.Cookies["awsccc"] = httputil.Awsccc()

	redirect := fmt.Sprintf("%s/start/#/device?user_code=%s", r.Cfg.ViewBase, r.UserCode)
	url := fmt.Sprintf("%s/login?directory_id=view&redirect_url=%s", r.Cfg.PortalBase, redirect)

	h := map[string]string{
		"Accept":       "application/json, text/plain, */*",
		"Content-Type": "application/json",
		"Origin":       r.Cfg.ViewBase,
		"Referer":      r.Cfg.ViewBase + "/",
		"User-Agent":   r.Identity.UA,
	}

	body, _, respH, err := r.DoGet(url, h)
	if err != nil {
		return err
	}
	httputil.SaveCookies(r.Cookies, respH)

	var data map[string]interface{}
	json.Unmarshal(body, &data)

	rurl, _ := data["redirectUrl"].(string)
	if strings.Contains(rurl, "workflowStateHandle=") {
		r.WorkflowHandle = httputil.SplitAfter(rurl, "workflowStateHandle=")
	}
	if csrf, ok := data["csrfToken"].(string); ok {
		r.Cookies["loginCsrfToken"] = csrf
	}
	if r.WorkflowHandle == "" {
		return fmt.Errorf("Portal 未返回 workflow handle")
	}

	loginURL := fmt.Sprintf("%s/platform/%s/login?workflowStateHandle=%s",
		r.Cfg.SigninBase, r.Cfg.DirectoryID, r.WorkflowHandle)
	return r.FetchD2CToken(r.Cfg.SigninBase, loginURL)
}

// Step5WorkflowInit 工作流初始化
func (r *Registrar) Step5WorkflowInit() error {
	log.Println("[5] 工作流初始化")
	api := fmt.Sprintf("%s/platform/%s/api/execute", r.Cfg.SigninBase, r.Cfg.DirectoryID)
	ref := fmt.Sprintf("%s/platform/%s/login?workflowStateHandle=%s",
		r.Cfg.SigninBase, r.Cfg.DirectoryID, r.WorkflowHandle)

	fp := r.GenFP("signin", "first_load", 0, "")
	rid := NewUUID()
	h := r.BuildHeaders(ref, r.Cfg.SigninBase)
	h["x-amzn-requestid"] = rid
	h["x-amz-date"] = GmtDate()
	h["priority"] = "u=1, i"

	body, _, respH, err := r.DoPostRaw(api, map[string]interface{}{
		"stepId": "", "workflowStateHandle": r.WorkflowHandle,
		"inputs":    []interface{}{map[string]string{"input_type": "FingerPrintRequestInput", "fingerPrint": fp}},
		"requestId": rid,
	}, h)
	if err != nil {
		return err
	}
	httputil.SaveCookies(r.Cookies, respH)

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	if wh, ok := data["workflowStateHandle"].(string); ok {
		r.WorkflowHandle = wh
	}

	if data["stepId"] == "start" {
		fp = r.GenFP("signin", "PageLoad", 0, "")
		rid = NewUUID()
		h = r.BuildHeaders(ref, r.Cfg.SigninBase)
		h["x-amzn-requestid"] = rid
		h["x-amz-date"] = GmtDate()
		h["priority"] = "u=1, i"

		body, _, respH, err = r.DoPostRaw(api, map[string]interface{}{
			"stepId": "start", "workflowStateHandle": r.WorkflowHandle,
			"inputs":    []interface{}{map[string]string{"input_type": "FingerPrintRequestInput", "fingerPrint": fp}},
			"requestId": rid,
		}, h)
		if err != nil {
			return err
		}
		httputil.SaveCookies(r.Cookies, respH)
		json.Unmarshal(body, &data)
		if wh, ok := data["workflowStateHandle"].(string); ok {
			r.WorkflowHandle = wh
		}
	}
	return nil
}

// NewUUID 生成 UUID
func NewUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// GmtDate 生成 GMT 日期字符串
func GmtDate() string {
	return time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
}
