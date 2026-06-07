package email

// TempEmailService 临时邮箱服务接口
type TempEmailService interface {
	// Create 创建临时邮箱，返回邮箱地址
	Create() string

	// WaitForCode 等待验证码，返回验证码字符串
	WaitForCode(timeoutSec, intervalSec int) (string, error)

	// GetAddress 获取当前邮箱地址
	GetAddress() string
}

// NewMoEmailService 创建 MoEmail 临时邮箱服务
func NewMoEmailService(baseURL, apiKey, proxy, chromeVer string) TempEmailService {
	return NewMoEmailProvider(baseURL, apiKey, proxy, chromeVer)
}
