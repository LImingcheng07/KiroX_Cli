package crypto

import (
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"

	fhttp "github.com/bogdanfinn/fhttp"
	httputil "reg_go/internal/http"
)

const (
	delta      uint32 = 0x9E3779B9
	mask       uint32 = 0xFFFFFFFF
	identifier        = "ECdITeCs"
	fallbackVer       = "4.0.0"
)

var (
	fallbackKey = [4]uint32{1888420705, 2576816180, 2347232058, 874813317}

	cacheMu          sync.Mutex
	cachedKey        *[4]uint32
	cachedVersion    string
	cachedIdentifier string
)

// RefreshAppJSConfig 从 app.js 刷新 XXTEA 密钥和 TES 版本
func RefreshAppJSConfig(proxy string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedKey != nil {
		return
	}

	js := fetchAppJS(proxy)
	if js != "" {
		key, ident, ver := extractFromAppJS(js)
		if key != nil {
			cachedKey = key
		}
		if ident != "" {
			cachedIdentifier = ident
		}
		if ver != "" {
			cachedVersion = ver
		}
	}
	if cachedKey == nil {
		log.Println("[xxtea] 使用 fallback 密钥")
		k := fallbackKey
		cachedKey = &k
	}
	if cachedVersion == "" {
		cachedVersion = fallbackVer
	}
	if cachedIdentifier == "" {
		cachedIdentifier = identifier
	}
}

// GetTESVersion 获取当前 TES 版本
func GetTESVersion() string {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedVersion != "" {
		return cachedVersion
	}
	return fallbackVer
}

// GetIdentifier 获取当前 identifier
func GetIdentifier() string {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedIdentifier != "" {
		return cachedIdentifier
	}
	return identifier
}

// GetActiveKey 获取当前 XXTEA 密钥
func GetActiveKey() [4]uint32 {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cachedKey != nil {
		return *cachedKey
	}
	return fallbackKey
}

// EncryptFingerprint 加密指纹 JSON 字符串
// 流程: JSON -> CRC32前缀 -> XXTEA加密 -> base64编码 -> "ECdITeCs:" + 结果
func EncryptFingerprint(jsonStr string) string {
	crc := crc32.ChecksumIEEE([]byte(jsonStr))
	crcHex := fmt.Sprintf("%08X", crc)
	plaintext := crcHex + "#" + jsonStr

	key := GetActiveKey()
	encrypted := xxteaEncrypt(plaintext, key)
	encoded := base64.StdEncoding.EncodeToString(encrypted)
	return GetIdentifier() + ":" + encoded
}

// xxteaEncrypt XXTEA 分组密码加密
func xxteaEncrypt(plaintext string, key [4]uint32) []byte {
	if len(plaintext) == 0 {
		return nil
	}

	// 字符串 -> uint32 数组 (little-endian)
	n := (len(plaintext) + 3) / 4
	v := make([]uint32, n)
	for i := 0; i < n; i++ {
		var b0, b1, b2, b3 byte
		if 4*i < len(plaintext) {
			b0 = plaintext[4*i]
		}
		if 4*i+1 < len(plaintext) {
			b1 = plaintext[4*i+1]
		}
		if 4*i+2 < len(plaintext) {
			b2 = plaintext[4*i+2]
		}
		if 4*i+3 < len(plaintext) {
			b3 = plaintext[4*i+3]
		}
		v[i] = uint32(b0) | uint32(b1)<<8 | uint32(b2)<<16 | uint32(b3)<<24
	}

	// XXTEA 加密
	rounds := 6 + 52/n
	z := v[n-1]
	var total uint32

	for r := 0; r < rounds; r++ {
		total += delta
		e := (total >> 2) & 3
		for p := 0; p < n; p++ {
			y := v[(p+1)%n]
			part1 := (z >> 5) ^ (y << 2)
			part2 := (y >> 3) ^ (z << 4)
			group1 := part1 + part2
			part3 := total ^ y
			part4 := key[(uint32(p)&3)^e] ^ z
			group2 := part3 + part4
			mx := group1 ^ group2
			v[p] += mx
			z = v[p]
		}
	}

	// uint32 数组 -> 字节序列 (little-endian)
	result := make([]byte, n*4)
	for i, val := range v {
		result[4*i] = byte(val)
		result[4*i+1] = byte(val >> 8)
		result[4*i+2] = byte(val >> 16)
		result[4*i+3] = byte(val >> 24)
	}
	return result
}

// fetchAppJS 下载 signin.aws app.js
func fetchAppJS(proxy string) string {
	client := httputil.NewTLSClient(proxy, true, "144.0.0.0")
	req, _ := fhttp.NewRequest("GET", "https://us-east-1.signin.aws/assets/js/app.js", nil)
	httputil.SetHeaders(req, map[string]string{
		"User-Agent":     httputil.DefaultUA(),
		"Accept":         "*/*",
		"Accept-Language": "en-US,en;q=0.9",
		"Referer":        "https://us-east-1.signin.aws/",
		"sec-ch-ua":      httputil.DefaultSecUA(),
		"sec-fetch-dest": "script",
		"sec-fetch-mode": "no-cors",
		"sec-fetch-site": "same-origin",
	})
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[xxtea] 下载 app.js 失败: %v", err)
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// extractFromAppJS 从 app.js 提取密钥、identifier 和版本
func extractFromAppJS(js string) (*[4]uint32, string, string) {
	var key *[4]uint32
	var ident, ver string

	// 提取密钥: var e=[2576816180,"ECdITeCs",874813317,1888420705,2347232058]
	re := regexp.MustCompile(`var\s+\w+\s*=\s*\[(\d+),\s*"([A-Za-z0-9]+)",\s*(\d+),\s*(\d+),\s*(\d+)\]`)
	m := re.FindStringSubmatch(js)
	if len(m) == 6 {
		nums := make([]uint32, 4)
		for i, idx := range []int{1, 3, 4, 5} {
			v, _ := strconv.ParseUint(m[idx], 10, 32)
			nums[i] = uint32(v)
		}
		// material = [e[3], e[0], e[4], e[2]]
		k := [4]uint32{nums[2], nums[0], nums[3], nums[1]}
		key = &k
		ident = m[2]
	}

	// 提取 TES version (FWCIM_VERSION)
	reVer := regexp.MustCompile(`FWCIM_VERSION\s*=\s*"(\d+\.\d+\.\d+)"`)
	vm := reVer.FindStringSubmatch(js)
	if len(vm) == 2 {
		ver = vm[1]
	}

	return key, ident, ver
}

// DecryptFingerprint 解密指纹字符串 (调试用)
func DecryptFingerprint(encrypted string) (string, error) {
	parts := strings.SplitN(encrypted, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("格式错误: 缺少 identifier 前缀")
	}
	if parts[0] != GetIdentifier() {
		return "", fmt.Errorf("未知 identifier: %s", parts[0])
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	raw := xxteaDecrypt(data, GetActiveKey())
	if idx := strings.Index(raw[:min(16, len(raw))], "#"); idx >= 0 {
		return raw[idx+1:], nil
	}
	return raw, nil
}

// xxteaDecrypt XXTEA 解密
func xxteaDecrypt(data []byte, key [4]uint32) string {
	n := len(data) / 4
	if n < 2 {
		return ""
	}
	v := make([]uint32, n)
	for i := 0; i < n; i++ {
		v[i] = uint32(data[4*i]) | uint32(data[4*i+1])<<8 |
			uint32(data[4*i+2])<<16 | uint32(data[4*i+3])<<24
	}

	rounds := 6 + 52/n
	total := uint32(rounds) * delta
	y := v[0]

	for r := 0; r < rounds; r++ {
		e := (total >> 2) & 3
		for p := n - 1; p >= 0; p-- {
			z := v[(p-1+n)%n]
			part1 := (z >> 5) ^ (y << 2)
			part2 := (y >> 3) ^ (z << 4)
			group1 := part1 + part2
			part3 := total ^ y
			part4 := key[(uint32(p)&3)^e] ^ z
			group2 := part3 + part4
			mx := group1 ^ group2
			v[p] -= mx
			y = v[p]
		}
		total -= delta
	}

	var sb strings.Builder
	for _, val := range v {
		sb.WriteByte(byte(val))
		sb.WriteByte(byte(val >> 8))
		sb.WriteByte(byte(val >> 16))
		sb.WriteByte(byte(val >> 24))
	}
	return strings.TrimRight(sb.String(), "\x00")
}
