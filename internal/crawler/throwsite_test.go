package crawler

import (
	"strings"
	"testing"

	"github.com/anurag/nexus/model"
)

func TestExtractThrowSites_ThrowException(t *testing.T) {
	src := `package com.example;

import com.example.exceptions.CartNotFoundException;

public class CartService {
    public void getCart(String id) {
        if (id == null) {
            throw new CartNotFoundException("Cart CART_NOT_FOUND for id: " + id);
        }
    }
}`
	imports := []string{"com.example.exceptions.CartNotFoundException"}
	methods := []model.Method{{Name: "getCart", Line: 6}}

	sites := extractThrowSites(src, imports, methods)
	if len(sites) != 1 {
		t.Fatalf("expected 1 throw site, got %d", len(sites))
	}
	s := sites[0]
	if s.Kind != "THROW" {
		t.Errorf("expected THROW, got %s", s.Kind)
	}
	if s.ExceptionFQN != "com.example.exceptions.CartNotFoundException" {
		t.Errorf("expected FQN com.example.exceptions.CartNotFoundException, got %s", s.ExceptionFQN)
	}
	if s.ErrorCode != "CART_NOT_FOUND" {
		t.Errorf("expected error code CART_NOT_FOUND, got %s", s.ErrorCode)
	}
	if s.Method != "getCart" {
		t.Errorf("expected method getCart, got %s", s.Method)
	}
	if s.Line < 7 || s.Line > 9 {
		t.Errorf("expected line around 8, got %d", s.Line)
	}
}

func TestExtractThrowSites_LogError(t *testing.T) {
	src := `package com.example;

public class PaymentService {
    public void process() {
        log.error("Payment PAYMENT_FAILED processing order");
    }
}`
	methods := []model.Method{{Name: "process", Line: 4}}

	sites := extractThrowSites(src, nil, methods)
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	s := sites[0]
	if s.Kind != "LOG_ERROR" {
		t.Errorf("expected LOG_ERROR, got %s", s.Kind)
	}
	if s.ErrorCode != "PAYMENT_FAILED" {
		t.Errorf("expected PAYMENT_FAILED, got %s", s.ErrorCode)
	}
	if s.Method != "process" {
		t.Errorf("expected method process, got %s", s.Method)
	}
}

func TestExtractThrowSites_LogWarn(t *testing.T) {
	src := `package com.example;

public class CacheService {
    public void evict() {
        logger.warn("Cache CACHE_MISS eviction triggered for region");
    }
}`
	methods := []model.Method{{Name: "evict", Line: 4}}

	sites := extractThrowSites(src, nil, methods)
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	if sites[0].Kind != "LOG_WARN" {
		t.Errorf("expected LOG_WARN, got %s", sites[0].Kind)
	}
	if sites[0].ErrorCode != "CACHE_MISS" {
		t.Errorf("expected CACHE_MISS, got %s", sites[0].ErrorCode)
	}
}

func TestExtractThrowSites_ErrorCodeDetection(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"CART_NOT_FOUND for user", "CART_NOT_FOUND"},
		{"Error code: ORDER_PROCESSING_FAILED", "ORDER_PROCESSING_FAILED"},
		{"simple message no code", ""},
		{"ABC too short", ""},
		{"SINGLE not enough", ""},
	}
	for _, tc := range tests {
		got := extractErrorCode(tc.msg)
		if got != tc.want {
			t.Errorf("extractErrorCode(%q) = %q, want %q", tc.msg, got, tc.want)
		}
	}
}

func TestMessageTokens(t *testing.T) {
	tokens := messageTokens("Payment processing failed for customer {customerId}")
	if len(tokens) == 0 {
		t.Fatal("expected tokens, got none")
	}
	for _, tok := range tokens {
		if throwStopwords[tok] {
			t.Errorf("stopword %q should be filtered", tok)
		}
		if len(tok) < 3 {
			t.Errorf("token %q too short, should be filtered", tok)
		}
	}
	found := false
	for _, tok := range tokens {
		if tok == "payment" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'payment' in tokens")
	}
}

func TestMessageTokens_PlaceholderStripping(t *testing.T) {
	tokens := messageTokens("Order %s failed with ${error.code} retry %d")
	for _, tok := range tokens {
		if strings.Contains(tok, "%") || strings.Contains(tok, "$") || strings.Contains(tok, "{") {
			t.Errorf("placeholder not stripped: %q", tok)
		}
	}
}

func TestMessageTokens_MaxTokens(t *testing.T) {
	long := "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec"
	tokens := messageTokens(long)
	if len(tokens) > 12 {
		t.Errorf("expected max 12 tokens, got %d", len(tokens))
	}
}

func TestExtractThrowSites_MethodAttribution(t *testing.T) {
	src := `package com.example;

public class MyService {
    public void methodA() {
        throw new RuntimeException("error in A");
    }
    public void methodB() {
        throw new IllegalStateException("error in B");
    }
}`
	methods := []model.Method{
		{Name: "methodA", Line: 4},
		{Name: "methodB", Line: 7},
	}

	sites := extractThrowSites(src, nil, methods)
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	if sites[0].Method != "methodA" {
		t.Errorf("first throw should be in methodA, got %s", sites[0].Method)
	}
	if sites[1].Method != "methodB" {
		t.Errorf("second throw should be in methodB, got %s", sites[1].Method)
	}
}

func TestExtractThrowSites_Deduplication(t *testing.T) {
	src := `package com.example;

public class DupService {
    public void check() {
        throw new RuntimeException("duplicate");
        throw new RuntimeException("duplicate");
    }
}`
	methods := []model.Method{{Name: "check", Line: 4}}

	sites := extractThrowSites(src, nil, methods)
	// Same line impossible in real code, but the two throws are on different lines
	// Both should appear since they have different line numbers
	for i := 0; i < len(sites); i++ {
		for j := i + 1; j < len(sites); j++ {
			if sites[i].Kind == sites[j].Kind && sites[i].ExceptionFQN == sites[j].ExceptionFQN && sites[i].Line == sites[j].Line {
				t.Error("duplicate throw site not deduplicated")
			}
		}
	}
}

func TestExtractThrowSites_MaxCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("package com.example;\npublic class Big {\n    public void big() {\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("        throw new RuntimeException(\"error message NUMBER_")
		sb.WriteString(itoa(i))
		sb.WriteString("\");\n")
	}
	sb.WriteString("    }\n}\n")
	src := sb.String()
	methods := []model.Method{{Name: "big", Line: 3}}

	sites := extractThrowSites(src, nil, methods)
	if len(sites) > maxThrowSites {
		t.Errorf("expected max %d sites, got %d", maxThrowSites, len(sites))
	}
}

func TestExtractThrowSites_UnresolvedImport(t *testing.T) {
	src := `package com.example;

public class NoImport {
    public void fail() {
        throw new CustomException("oops");
    }
}`
	methods := []model.Method{{Name: "fail", Line: 4}}

	sites := extractThrowSites(src, nil, methods)
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}
	if sites[0].ExceptionFQN != "CustomException" {
		t.Errorf("expected bare name CustomException, got %s", sites[0].ExceptionFQN)
	}
}

func TestExtractThrowSites_EmptyLogDropped(t *testing.T) {
	src := `package com.example;

public class Quiet {
    public void quiet() {
        log.error(ex);
    }
}`
	methods := []model.Method{{Name: "quiet", Line: 4}}

	sites := extractThrowSites(src, nil, methods)
	if len(sites) != 0 {
		t.Errorf("expected 0 sites for log without string literal, got %d", len(sites))
	}
}

func TestJavaParser_ThrowSitesIntegration(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

import com.example.errors.OrderException;

public class OrderService {
    public void placeOrder(String id) {
        if (id == null) {
            throw new OrderException("ORDER_INVALID null id");
        }
        log.error("Order ORDER_PROCESSING_FAILED for id: " + id);
    }
}`)

	units, err := jp.ParseFile("src/main/java/OrderService.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if len(units[0].ThrowSites) != 2 {
		t.Fatalf("expected 2 throw sites, got %d", len(units[0].ThrowSites))
	}
	if units[0].ThrowSites[0].Kind != "THROW" {
		t.Errorf("first site should be THROW, got %s", units[0].ThrowSites[0].Kind)
	}
	if units[0].ThrowSites[1].Kind != "LOG_ERROR" {
		t.Errorf("second site should be LOG_ERROR, got %s", units[0].ThrowSites[1].Kind)
	}
}

func TestJavaParser_ThrowSitesSkippedForTests(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

public class OrderServiceTest {
    public void testPlaceOrder() {
        throw new RuntimeException("expected in test");
    }
}`)

	units, err := jp.ParseFile("src/test/java/OrderServiceTest.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if len(units[0].ThrowSites) != 0 {
		t.Errorf("expected 0 throw sites for test file, got %d", len(units[0].ThrowSites))
	}
}
