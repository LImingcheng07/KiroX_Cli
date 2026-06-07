package email

import "testing"

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "english", text: "Your verification code: 654321", want: "654321"},
		{name: "chinese", text: "验证码：234567", want: "234567"},
		{name: "blacklist", text: "Your verification code: 123456", want: ""},
		{name: "fallback", text: "Use 345678 to continue", want: "345678"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractCode(tt.text); got != tt.want {
				t.Fatalf("ExtractCode() = %q, want %q", got, tt.want)
			}
		})
	}
}
