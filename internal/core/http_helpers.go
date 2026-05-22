package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	httputil "reg_go/internal/http"
)

// BuildHeaders 构建通用请求头
func (r *Registrar) BuildHeaders(referer, origin string) map[string]string {
	h := map[string]string{
		"Accept":              "application/json, text/plain, */*",
		"Accept-Language":     "zh-CN,zh;q=0.9,en;q=0.8",
		"Accept-Encoding":    "gzip, deflate, br",
		"Content-Type":       "application/json",
		"User-Agent":         r.Identity.UA,
		"sec-ch-ua":          r.Identity.SecUA,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
	}
	if referer != "" {
		h["Referer"] = referer
	}
	if origin != "" {
		h["Origin"] = origin
	}
	if len(r.Cookies) > 0 {
		h["Cookie"] = r.CookieString()
	}
	return h
}

// BuildProfileHeaders 构建 profile 页面请求头
func (r *Registrar) BuildProfileHeaders(referer string) map[string]string {
	h := map[string]string{
		"Accept":              "*/*",
		"Accept-Language":     "zh-CN,zh;q=0.9,en;q=0.8",
		"Content-Type":       "application/json;charset=UTF-8",
		"User-Agent":         r.Identity.UA,
		"Origin":             r.Cfg.ProfileBase,
		"Referer":            referer,
		"sec-ch-ua":          r.Identity.SecUA,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"priority":           "u=1, i",
	}
	keys := []string{"awsccc", "aws-user-profile-ubid", "i18next"}
	if _, ok := r.Cookies["awsd2c-token"]; ok {
		keys = append(keys, "awsd2c-token", "awsd2c-token-c")
	}
	var parts []string
	for _, k := range keys {
		if v, ok := r.Cookies[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if len(parts) > 0 {
		h["Cookie"] = strings.Join(parts, "; ")
	}
	return h
}

// CookieString 将 cookies 拼接为字符串
func (r *Registrar) CookieString() string {
	var parts []string
	for k, v := range r.Cookies {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "; ")
}

// FetchD2CToken 获取 D2C Token
func (r *Registrar) FetchD2CToken(origin, referer string) error {
	headers := map[string]string{
		"Accept":              "*/*",
		"Content-Type":       "application/json",
		"User-Agent":         r.Identity.UA,
		"Origin":             origin,
		"Referer":            referer,
		"sec-ch-ua":          r.Identity.SecUA,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "cross-site",
		"priority":           "u=1, i",
	}
	var parts []string
	if v, ok := r.Cookies["awsccc"]; ok {
		parts = append(parts, "awsccc="+v)
	}
	if old, ok := r.Cookies["awsd2c-token"]; ok {
		parts = append(parts, "awsd2c-token="+old, "awsd2c-token-c="+old)
	}
	if len(parts) > 0 {
		headers["Cookie"] = strings.Join(parts, "; ")
	}

	payload := map[string]interface{}{}
	if old, ok := r.Cookies["awsd2c-token"]; ok {
		payload["token"] = old
	}

	body, respHeaders, err := r.DoPost("https://vs.aws.amazon.com/token", payload, headers)
	if err != nil {
		return err
	}
	httputil.SaveCookies(r.Cookies, respHeaders)

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	if tok, ok := data["token"].(string); ok && tok != "" {
		r.Cookies["awsd2c-token"] = tok
		r.Cookies["awsd2c-token-c"] = tok
		// 从 JWT 中提取 visitor ID
		jwtParts := strings.Split(tok, ".")
		if len(jwtParts) >= 2 {
			pad := jwtParts[1]
			if m := len(pad) % 4; m != 0 {
				pad += strings.Repeat("=", 4-m)
			}
			pad = strings.ReplaceAll(pad, "-", "+")
			pad = strings.ReplaceAll(pad, "_", "/")
			if decoded, err := base64.StdEncoding.DecodeString(pad); err == nil {
				var p map[string]interface{}
				if json.Unmarshal(decoded, &p) == nil {
					if vid, ok := p["vid"].(string); ok {
						r.VisitorID = vid
					}
				}
			}
		}
	}
	return nil
}
