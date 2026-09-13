package crawler

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func TestJavaParserBasicClass(t *testing.T) {
	jp := NewJavaParser()
	if jp.Platform() != model.Java {
		t.Fatal("wrong platform")
	}

	src := []byte(`package com.example;

public class OrderService {
    private String name;

    public void createOrder(String id) {
        // business logic
    }

    public String getOrder(String id) {
        return id;
    }
}`)

	units, err := jp.ParseFile("OrderService.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Name != "OrderService" {
		t.Fatalf("expected OrderService, got %s", units[0].Name)
	}
	if units[0].Stereotype != "service" {
		t.Fatalf("expected stereotype 'service', got %q", units[0].Stereotype)
	}
	if len(units[0].Methods) < 2 {
		t.Fatalf("expected at least 2 methods, got %d", len(units[0].Methods))
	}
}

func TestJavaParserRESTController(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/orders")
public class OrderController {
    @GetMapping("/list")
    public List<Order> listOrders() {
        return null;
    }

    @PostMapping("/create")
    public Order createOrder(@RequestBody OrderRequest req) {
        return null;
    }
}`)

	units, err := jp.ParseFile("OrderController.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if len(units[0].Endpoints) == 0 {
		t.Fatal("expected endpoints to be detected")
	}
}

func TestJavaParserKafka(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

import org.springframework.kafka.annotation.KafkaListener;

public class OrderConsumer {
    @KafkaListener(topics = "order-events")
    public void handleOrderEvent(String event) {
    }
}`)

	units, err := jp.ParseFile("OrderConsumer.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if len(units[0].KafkaConsumes) == 0 {
		t.Fatal("expected kafka_consumes to be detected")
	}
	if units[0].KafkaConsumes[0] != "order-events" {
		t.Fatalf("expected topic 'order-events', got %s", units[0].KafkaConsumes[0])
	}
}

func TestJavaParserInterface(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

public interface OrderRepository {
    Order findById(String id);
    void save(Order order);
}`)

	units, err := jp.ParseFile("OrderRepository.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Type != "interface_declaration" {
		t.Fatalf("expected interface_declaration, got %s", units[0].Type)
	}
	if units[0].Stereotype != "repository" {
		t.Fatalf("expected stereotype 'repository', got %q", units[0].Stereotype)
	}
}

func TestJavaParserFieldExtraction(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

import lombok.Data;
import java.util.List;

@Data
public class AemPushNotification {
    private String type;
    private String title;
    private String description;
    private Long timestamp;
    private List<String> tags;
    private boolean active;
}`)

	units, err := jp.ParseFile("AemPushNotification.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	u := units[0]
	if len(u.Fields) != 6 {
		t.Fatalf("expected 6 fields, got %d: %+v", len(u.Fields), u.Fields)
	}

	expected := map[string]string{
		"type": "String", "title": "String", "description": "String",
		"timestamp": "Long", "tags": "List<String>", "active": "boolean",
	}
	for _, f := range u.Fields {
		want, ok := expected[f.Name]
		if !ok {
			t.Errorf("unexpected field %q", f.Name)
			continue
		}
		if f.Type != want {
			t.Errorf("field %s: expected type %q, got %q", f.Name, want, f.Type)
		}
	}
}

func TestJavaParserImportExtraction(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

import com.example.platform.contentservices.appcore.facades.AemNotificationFacade;
import com.example.platform.contentservices.appcore.services.map.NotificationMapper;
import java.util.List;

public class NotificationService {
    private final AemNotificationFacade facade;
}`)

	units, err := jp.ParseFile("NotificationService.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if len(units[0].Imports) != 3 {
		t.Fatalf("expected 3 imports, got %d: %v", len(units[0].Imports), units[0].Imports)
	}
	if units[0].Imports[0] != "com.example.platform.contentservices.appcore.facades.AemNotificationFacade" {
		t.Errorf("unexpected first import: %s", units[0].Imports[0])
	}
}

func TestJavaParserConfigFieldAnnotations(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class NotificationExpirationConfig {
    private long ttlHours = 24;
    private long cacheTtlHours = 24;
    private String orderNotificationType = "commerce";
}`)

	units, err := jp.ParseFile("NotificationExpirationConfig.java", src)
	if err != nil {
		t.Fatal(err)
	}
	u := units[0]
	if len(u.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %+v", len(u.Fields), u.Fields)
	}
	fieldNames := make(map[string]bool)
	for _, f := range u.Fields {
		fieldNames[f.Name] = true
	}
	for _, name := range []string{"ttlHours", "cacheTtlHours", "orderNotificationType"} {
		if !fieldNames[name] {
			t.Errorf("missing field %q", name)
		}
	}
}

func TestJavaParserExtendsImplements(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

public class NotificationServiceImpl extends BaseService implements NotificationHandler, Serializable {
    private String name;

    public void handle() {}
}`)

	units, err := jp.ParseFile("NotificationServiceImpl.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	u := units[0]
	if u.Extends != "BaseService" {
		t.Errorf("expected Extends='BaseService', got %q", u.Extends)
	}
	if len(u.Implements) != 2 {
		t.Fatalf("expected 2 implements, got %d: %v", len(u.Implements), u.Implements)
	}
	if u.Implements[0] != "NotificationHandler" {
		t.Errorf("expected Implements[0]='NotificationHandler', got %q", u.Implements[0])
	}
	if u.Implements[1] != "Serializable" {
		t.Errorf("expected Implements[1]='Serializable', got %q", u.Implements[1])
	}
}

func TestJavaParserInterfaceExtends(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

public interface OrderRepository extends JpaRepository, CustomRepo {
    Order findById(String id);
}`)

	units, err := jp.ParseFile("OrderRepository.java", src)
	if err != nil {
		t.Fatal(err)
	}
	u := units[0]
	if len(u.Implements) != 2 {
		t.Fatalf("expected 2 interface extends, got %d: %v", len(u.Implements), u.Implements)
	}
}

func TestJavaParserMethodCallExtraction(t *testing.T) {
	jp := NewJavaParser()
	src := []byte(`package com.example;

public class NotificationService {
    private final AemNotificationFacade notificationFacade;
    private final NotificationMapper mapper;

    public Object getNotifications(String id) {
        Object raw = notificationFacade.getNotificationContent(id);
        Object mapped = mapper.mapToNotificationResponse(raw);
        log.info("done");
        return mapped;
    }

    public void invalidate(String id) {
        notificationFacade.invalidateCache(id);
    }
}`)

	units, err := jp.ParseFile("NotificationService.java", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	u := units[0]

	if len(u.Methods) < 2 {
		t.Fatalf("expected at least 2 methods, got %d", len(u.Methods))
	}

	getNotifs := u.Methods[0]
	if getNotifs.Name != "getNotifications" {
		t.Fatalf("expected first method 'getNotifications', got %q", getNotifs.Name)
	}
	if len(getNotifs.Calls) < 2 {
		t.Fatalf("expected at least 2 calls in getNotifications, got %d: %+v", len(getNotifs.Calls), getNotifs.Calls)
	}

	callMap := map[string]string{}
	for _, c := range getNotifs.Calls {
		callMap[c.Target] = c.Method
	}
	if callMap["notificationFacade"] != "getNotificationContent" {
		t.Errorf("expected notificationFacade.getNotificationContent, got %v", callMap)
	}
	if callMap["mapper"] != "mapToNotificationResponse" {
		t.Errorf("expected mapper.mapToNotificationResponse, got %v", callMap)
	}

	invalidate := u.Methods[1]
	if len(invalidate.Calls) < 1 {
		t.Fatalf("expected at least 1 call in invalidate, got %d", len(invalidate.Calls))
	}
	if invalidate.Calls[0].Target != "notificationFacade" || invalidate.Calls[0].Method != "invalidateCache" {
		t.Errorf("unexpected call: %+v", invalidate.Calls[0])
	}
}

func TestExtractAnnotationValue(t *testing.T) {
	cases := map[string]struct{ ann, key, want string }{
		"topics":  {`@KafkaListener(topics = "order-events")`, "topics", "order-events"},
		"value":   {`@GetMapping(value = "/list")`, "value", "/list"},
		"noquote": {`@GetMapping("/list")`, "value", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := extractAnnotationValue(tc.ann, tc.key)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractAnnotationSingleValue(t *testing.T) {
	cases := map[string]struct{ ann, want string }{
		"simple":    {`@GetMapping("/list")`, "/list"},
		"no_parens": {`@GetMapping`, ""},
		"with_key":  {`@GetMapping(value = "/list")`, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := extractAnnotationSingleValue(tc.ann)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
