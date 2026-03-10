// dnsqueue is a library for queueing up a large number of DNS requests.
package dnsqueue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/miekg/dns"
)

// Request contains data for making a DNS request
type Request struct {
	Destination     string
	RecordType      string
	RecordName      string
	VerifySignature bool
	Context         context.Context

	exit bool
}

// Answer contains a single answer returned by a DNS server.
type Answer struct {
	Ttl    uint32
	Name   string
	String string
}

// Result contains metadata relating to a set of DNS server results.
type Result struct {
	Request      Request
	Duration     time.Duration
	Answers      []Answer
	ResponseCode int
	Error        string
}

// Queue contains methods and state for setting up a request queue.
type Queue struct {
	Requests    chan *Request
	Results     chan *Result
	WorkerCount int
	Quit        chan bool
}

// StartQueue starts a new queue with max length of X with worker count Y.
func StartQueue(size, workers int) (q *Queue) {
	q = &Queue{
		Requests:    make(chan *Request, size),
		Results:     make(chan *Result, size),
		WorkerCount: workers,
	}
	for i := 0; i < q.WorkerCount; i++ {
		go startWorker(q.Requests, q.Results)
	}
	return
}

// Queue.Add adds a request to the queue. Only blocks if queue is full.
func (q *Queue) Add(dest, record_type, record_name string) {
	q.AddWithContext(context.Background(), dest, record_type, record_name)
}

// Queue.AddWithContext adds a request to the queue with a cancelable context.
func (q *Queue) AddWithContext(ctx context.Context, dest, record_type, record_name string) {
	q.Requests <- &Request{
		Destination: dest,
		RecordType:  record_type,
		RecordName:  record_name,
		Context:     ctx,
	}
}

// Queue.SendDieSignal sends a signal to the workers that they can go home now.
func (q *Queue) SendCompletionSignal() {
	for i := 0; i < q.WorkerCount; i++ {
		q.Requests <- &Request{exit: true}
	}
}

// startWorker starts a thread to watch the request channel and populate result channel.
func startWorker(queue <-chan *Request, results chan<- *Result) {
	for request := range queue {
		if request.exit {
			return
		}
		result, err := SendQuery(request)
		if err != nil {
			log.Printf("Error sending query: %s", err)
		}
		results <- &result
	}
}

// Send a DNS query via UDP, configured by a Request object. If successful,
// stores response details in Result object, otherwise, returns Result object
// with an error string.
func SendQuery(request *Request) (result Result, err error) {
	result.Request = *request
	ctx := request.Context
	if ctx == nil {
		ctx = context.Background()
	}

	recordType, ok := dns.StringToType[request.RecordType]
	if !ok {
		result.Error = fmt.Sprintf("Invalid type: %s", request.RecordType)
		return result, errors.New(result.Error)
	}

	m := new(dns.Msg)
	if request.VerifySignature {
		m.SetEdns0(4096, true)
	}
	m.SetQuestion(request.RecordName, recordType)
	udpClient := &dns.Client{Net: "udp", Timeout: 4 * time.Second}
	in, rtt, err := udpClient.ExchangeContext(ctx, m, request.Destination)
	if err == nil && in != nil && in.Truncated {
		tcpClient := &dns.Client{Net: "tcp", Timeout: 4 * time.Second}
		in, rtt, err = tcpClient.ExchangeContext(ctx, m, request.Destination)
	}

	result.Duration = rtt
	if in != nil {
		result.ResponseCode = in.Rcode
	}
	if err != nil {
		result.Error = err.Error()
	} else {
		for _, rr := range in.Answer {
			answer := Answer{
				Ttl:    rr.Header().Ttl,
				Name:   rr.Header().Name,
				String: rr.String(),
			}
			result.Answers = append(result.Answers, answer)
		}
	}
	return result, err
}
