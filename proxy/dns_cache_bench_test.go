package proxy

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func BenchmarkDNSCache_Get(b *testing.B) {
	cache := newDNSCache(10000)

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
	})

	key := getCacheKey(msg)
	cache.set(key, msg, 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.get(key)
	}
}

func BenchmarkDNSCache_Set(b *testing.B) {
	cache := newDNSCache(10000)

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := getCacheKey(msg)
		cache.set(key, msg, 5*time.Minute)
	}
}

func BenchmarkGetCacheKey(b *testing.B) {
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getCacheKey(msg)
	}
}

func BenchmarkGetTTL(b *testing.B) {
	msg := new(dns.Msg)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getTTL(msg)
	}
}

func BenchmarkDNSCache_Concurrent(b *testing.B) {
	cache := newDNSCache(10000)

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
	})

	key := getCacheKey(msg)
	cache.set(key, msg, 5*time.Minute)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = cache.get(key)
		}
	})
}
