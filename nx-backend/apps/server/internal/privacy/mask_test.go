package privacy

import "testing"

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "mainland mobile", input: "13812345678", want: "138****5678"},
		{name: "trim whitespace", input: " 13912345678 ", want: "139****5678"},
		{name: "hyphenated mainland mobile", input: "138-1234-5678", want: "138****5678"},
		{name: "spaced mainland mobile", input: "138 1234 5678", want: "138****5678"},
		{name: "full width spaced mainland mobile", input: "138　1234　5678", want: "138****5678"},
		{name: "empty", input: "", want: ""},
		{name: "short nonstandard", input: "1234", want: "****"},
		{name: "long nonstandard", input: "1234567", want: "1****67"},
		{name: "unicode nonstandard", input: "号码未知", want: "****"},
		{name: "already masked", input: "138****5678", want: "138****5678"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskPhone(tt.input); got != tt.want {
				t.Fatalf("MaskPhone(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskPhonesInTextMasksEveryMainlandPhoneOnly(t *testing.T) {
	input := "客户张三：13812345678；备用 19987654321。订单202607231234不变，已脱敏138****5678，普通数字12345678901不变。"
	want := "客户张三：138****5678；备用 199****4321。订单202607231234不变，已脱敏138****5678，普通数字12345678901不变。"

	if got := MaskPhonesInText(input); got != want {
		t.Fatalf("MaskPhonesInText() = %q, want %q", got, want)
	}
}

func TestMaskPhonesInTextHandlesUnicodeBoundaries(t *testing.T) {
	input := "中文前缀138-1234-5678，空格 139 8765 4321，全角空格 137　0000　1234，连续13700001234。"
	want := "中文前缀138****5678，空格 139****4321，全角空格 137****1234，连续137****1234。"

	if got := MaskPhonesInText(input); got != want {
		t.Fatalf("MaskPhonesInText() = %q, want %q", got, want)
	}
}

func TestMaskPhonesInTextDoesNotDamageOrdinaryOrMaskedNumbers(t *testing.T) {
	input := "订单 2026-1234-5678，普通数字12345678901，长数字9138123456780，已脱敏138****5678。"
	if got := MaskPhonesInText(input); got != input {
		t.Fatalf("MaskPhonesInText() = %q, want unchanged %q", got, input)
	}
}
