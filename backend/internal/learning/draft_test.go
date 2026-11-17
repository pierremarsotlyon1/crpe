package learning

import "testing"

func TestGeneratedArithmeticIsCheckedExactly(t *testing.T) {
	for _, tc := range []struct{ expr, want string }{{"1/3 + 2/3", "1"}, {"(3.5-1)*2", "5"}, {"-4 / (2+3)", "-4/5"}, {"1/0", ""}, {"x()", ""}, {"1e999999999", ""}, {"0x7f", ""}, {"1/*2*/+9", ""}, {"5^7", ""}} {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := arithmetic(tc.expr)
			if tc.want == "" {
				if err == nil {
					t.Fatal("invalid expression accepted")
				}
				return
			}
			if err != nil || got.RatString() != tc.want {
				t.Fatal(got, err)
			}
		})
	}
}
