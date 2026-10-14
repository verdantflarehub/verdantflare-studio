package videotitle

import "testing"

func TestExactRenameBody(t *testing.T) {
	for _, body := range []string{`{"title":"纸船","expected_title_revision":0}`, `{"title":" 湖面漂流的纸船 ","expected_title_revision":2}`} {
		if !ValidBody([]byte(body)) {
			t.Fatalf("rejected valid rename: %s", body)
		}
	}
	for _, body := range []string{`{}`, `{"title":"纸船"}`, `{"title":"纸船","expected_title_revision":true}`, `{"title":"纸船","expected_title_revision":-1}`, `{"title":"纸船","expected_title_revision":1,"route":"other"}`, `{"title":"纸船","title":"另一个","expected_title_revision":1}`, `{"title":"a\nb","expected_title_revision":1}`, `{"title":"<b>x</b>","expected_title_revision":1}`} {
		if ValidBody([]byte(body)) {
			t.Fatalf("accepted invalid rename: %s", body)
		}
	}
}
