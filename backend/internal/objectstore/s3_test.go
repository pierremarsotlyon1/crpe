package objectstore

import "testing"

func TestConfigurationDoesNotFallBackOnPartialCredentials(t *testing.T) {
	store, err := FromEnv(func(string) string { return "" })
	if err != nil || store != nil {
		t.Fatalf("unconfigured storage: %v %v", store, err)
	}
	_, err = FromEnv(func(key string) string {
		if key == "S3_BUCKET" {
			return "private-copies"
		}
		return ""
	})
	if err == nil {
		t.Fatal("partial configuration must fail")
	}
}

func TestObjectKeysCannotTraverseOrInjectHeaders(t *testing.T) {
	for _, key := range []string{"", "/absolute", "copies/../secret", "copies//file", "copies/./file", "a\r\nheader", "a\\b"} {
		if err := validateKey(key); err == nil {
			t.Errorf("accepted unsafe key %q", key)
		}
	}
	if err := validateKey("copies/user-id/submission.jpg"); err != nil {
		t.Fatal(err)
	}
}
