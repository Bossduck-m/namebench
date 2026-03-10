package dnschecks

import (
	"errors"
	"testing"

	"github.com/google/namebench/dnsqueue"
	"github.com/miekg/dns"
)

func TestResolverValidatesDNSSEC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result dnsqueue.Result
		err    error
		want   bool
	}{
		{
			name:   "servfail means validating resolver",
			result: dnsqueue.Result{ResponseCode: dns.RcodeServerFailure},
			want:   true,
		},
		{
			name:   "success means non validating resolver",
			result: dnsqueue.Result{ResponseCode: dns.RcodeSuccess},
			want:   false,
		},
		{
			name:   "transport error is not validation",
			result: dnsqueue.Result{ResponseCode: dns.RcodeServerFailure},
			err:    errors.New("dial timeout"),
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolverValidatesDNSSEC(tt.result, tt.err)
			if got != tt.want {
				t.Fatalf("resolverValidatesDNSSEC() = %t, want %t", got, tt.want)
			}
		})
	}
}
