package dnschecks

import (
	"log"

	"github.com/google/namebench/dnsqueue"
	"github.com/miekg/dns"
)

func DnsSec(ip string) (ok bool, err error) {
	r := &dnsqueue.Request{
		Destination:     ip,
		RecordType:      "A",
		RecordName:      "www.dnssec-failed.org.",
		VerifySignature: true,
	}
	result, err := dnsqueue.SendQuery(r)
	ok = resolverValidatesDNSSEC(result, err)
	log.Printf("DnsSec for %s: %t (rcode=%s err=%v)", ip, ok, dns.RcodeToString[result.ResponseCode], err)
	return ok, err
}

// resolverValidatesDNSSEC treats SERVFAIL for dnssec-failed.org as evidence
// that the recursive resolver is validating DNSSEC and rejecting bogus data.
func resolverValidatesDNSSEC(result dnsqueue.Result, err error) bool {
	if err != nil {
		return false
	}
	return result.ResponseCode == dns.RcodeServerFailure
}
