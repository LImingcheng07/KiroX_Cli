package core

import (
	"encoding/json"
	"fmt"
	"log"
)

// Step15KiroExchange 用 auth code 兑换 Kiro access/refresh token
func (r *Registrar) Step15KiroExchange(code string) (map[string]interface{}, error) {
	log.Println("[15] Kiro ExchangeToken")

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth/callback", r.KiroRedirectPort)
	body, _, err := r.DoPost(r.Cfg.OIDCBase+"/token", map[string]interface{}{
		"clientId":     r.KiroClientID,
		"clientSecret": r.KiroClientSecret,
		"grantType":    "authorization_code",
		"code":         code,
		"redirectUri":  redirectURI,
		"codeVerifier": r.KiroCodeVerifier,
	}, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return nil, err
	}

	var rd map[string]interface{}
	if err := json.Unmarshal(body, &rd); err != nil {
		return nil, fmt.Errorf("token 响应解析失败: %s", string(body))
	}
	at, _ := rd["accessToken"].(string)
	if at == "" {
		return nil, fmt.Errorf("ExchangeToken 失败: %s", string(body))
	}
	rt, _ := rd["refreshToken"].(string)

	if len(at) > 40 {
		log.Printf("accessToken=%s...", at[:40])
	}
	if len(rt) > 40 {
		log.Printf("refreshToken=%s...", rt[:40])
	} else {
		log.Println("refreshToken=N/A")
	}

	return map[string]interface{}{
		"accessToken":  at,
		"refreshToken": rt,
		"expiresIn":    rd["expiresIn"],
		"tokenType":    rd["tokenType"],
	}, nil
}
