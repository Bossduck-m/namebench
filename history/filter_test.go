package history

import (
	"reflect"
	"testing"
)

func TestExternalHostnamesStripsPorts(t *testing.T) {
	t.Parallel()

	input := []string{
		"https://example.com:8443/path",
		"http://subdomain.example.co.uk:8080/test",
		"http://127.0.0.1:3000/internal",
	}

	got := ExternalHostnames(input)
	want := []string{"example.com", "subdomain.example.co.uk"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExternalHostnames() = %#v, want %#v", got, want)
	}
}

func TestExternalHostnamesSkipsNonWebSchemes(t *testing.T) {
	t.Parallel()

	input := []string{
		"chrome-extension://nkbihfbeogaeaoehlefnkodbefgpgknn/home.html",
		"edge-extension://abcdefghijklmnop/options.html",
		"file:///C:/Users/test/Desktop/readme.txt",
		"about:blank",
		"https://example.com/path",
	}

	got := ExternalHostnames(input)
	want := []string{"example.com"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExternalHostnames() = %#v, want %#v", got, want)
	}
}

func TestUniqRemovesNonAdjacentDuplicates(t *testing.T) {
	t.Parallel()

	input := []string{"a.example", "b.example", "a.example", "c.example", "b.example"}

	got := Uniq(input)
	want := []string{"a.example", "b.example", "c.example"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Uniq() = %#v, want %#v", got, want)
	}
}
