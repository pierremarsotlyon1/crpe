package learning

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

func TestReviewSchedule(t *testing.T) {
	for _, tc := range []struct {
		name         string
		streak, days int
	}{{"miss", 0, 1}, {"first", 1, 3}, {"second", 2, 7}, {"third", 3, 14}, {"fourth", 4, 30}, {"maximum", 5, 60}, {"cap", 9, 60}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reviewDelay(tc.streak); got != time.Duration(tc.days)*24*time.Hour {
				t.Fatal(got)
			}
		})
	}
}
func TestNumericGrading(t *testing.T) {
	c := AdminContent{Content: Content{ContentFields: ContentFields{Format: "numeric"}}, Reference: Reference{Answer: "2.5", Tolerance: 0.01}}
	for _, tc := range []struct {
		name, answer  string
		want, invalid bool
	}{{"decimal comma", "2,5", true, false}, {"within tolerance", "2.507", true, false}, {"outside tolerance", "2.52", false, false}, {"exact boundary", "2.51", true, false}, {"hex float", "0x1.4p1", false, true}, {"infinity", "+Inf", false, true}, {"not a number", "NaN", false, true}, {"overflow", "1e999", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := grade(c, tc.answer)
			if ok != tc.want || (err != nil) != tc.invalid {
				t.Fatalf("correct=%v err=%v", ok, err)
			}
		})
	}
}
func TestCleanImageRejectsTruncationAndStripsTrailingData(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(1, 1, color.White)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	if _, _, err := cleanImage(data[:len(data)-14]); err == nil {
		t.Fatal("accepted incomplete PNG")
	}
	out, mime, err := cleanImage(append(bytes.Clone(data), []byte("trailing-test")...))
	if err != nil || mime != "image/png" || bytes.Contains(out, []byte("trailing-test")) {
		t.Fatal(err, mime)
	}
}
