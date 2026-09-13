package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anurag/nexus/model"
)

func TestReadFileSignaturesBFS(t *testing.T) {
	// Create temp dir with fake Java files
	dir := t.TempDir()
	files := map[string]string{
		"NotificationService.java": `package com.example;
@Service
public class NotificationService {
    private final AemNotificationFacade facade;
    private final NotificationMapper mapper;
    public Mono<NotificationResponse> getNotificationContent() {}
}`,
		"AemNotificationFacade.java": `package com.example;
@Component
public class AemNotificationFacade {
    public Mono<AemNotificationContentResponse> getNotificationContent() {}
}`,
		"NotificationMapper.java": `package com.example;
public class NotificationMapper {
    public static NotificationResponse mapToNotificationResponse(AemNotificationContentResponse aem) {}
}`,
		"AemPushNotification.java": `package com.example;
@Data
public class AemPushNotification {
    private String type;
    private String title;
    private String description;
    private Long timestamp;
}`,
		"NotificationResponse.java": `package com.example;
@Data
public class NotificationResponse {
    private List<AppNotificationContent> notifications;
}`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Build ClassDetail list simulating indexed classes
	classes := []ClassDetail{
		{
			Name:         "NotificationService",
			File:         "NotificationService.java",
			Service:      "test-svc",
			Stereotype:   "service",
			Dependencies: []string{"AemNotificationFacade", "NotificationMapper"},
			Methods:      []string{"Mono<NotificationResponse> getNotificationContent()"},
		},
		{
			Name:         "AemNotificationFacade",
			File:         "AemNotificationFacade.java",
			Service:      "test-svc",
			Stereotype:   "component",
			Dependencies: []string{},
			Methods:      []string{"Mono<AemNotificationContentResponse> getNotificationContent()"},
		},
		{
			Name:       "NotificationMapper",
			File:       "NotificationMapper.java",
			Service:    "test-svc",
			Stereotype: "",
			Methods:    []string{"NotificationResponse mapToNotificationResponse(AemNotificationContentResponse aem)"},
		},
		{
			Name:       "AemPushNotification",
			File:       "AemPushNotification.java",
			Service:    "test-svc",
			Stereotype: "",
			Fields: []model.Field{
				{Name: "type", Type: "String"},
				{Name: "title", Type: "String"},
				{Name: "description", Type: "String"},
				{Name: "timestamp", Type: "Long"},
			},
		},
		{
			Name:       "NotificationResponse",
			File:       "NotificationResponse.java",
			Service:    "test-svc",
			Stereotype: "",
		},
	}

	excerpts := readFileSignatures(dir, classes, "")

	// Should have found NotificationService (seed) + its dependencies via BFS
	found := make(map[string]bool)
	for _, e := range excerpts {
		found[e.ClassName] = true
	}

	if !found["NotificationService"] {
		t.Error("BFS should include seed class NotificationService")
	}
	if !found["AemNotificationFacade"] {
		t.Error("BFS level 1 should include AemNotificationFacade (dependency of NotificationService)")
	}
	if !found["NotificationMapper"] {
		t.Error("BFS level 1 should include NotificationMapper (dependency of NotificationService)")
	}
	// AemPushNotification is NOT a dependency but its name is in the response fallback
	// or found via method signature traversal through AemNotificationContentResponse → contains "response"
	if !found["NotificationResponse"] {
		t.Error("BFS should include NotificationResponse (referenced in method signatures or name-matched)")
	}

	if len(excerpts) < 3 {
		t.Errorf("expected at least 3 excerpts from BFS, got %d", len(excerpts))
	}
}

func TestReadFileSignaturesBFSFallbackModels(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"MyService.java": `package com.example;
@Service
public class MyService {
    private final MyRepo repo;
}`,
		"MyRepo.java": `package com.example;
@Repository
public interface MyRepo {}`,
		"MyRequestDTO.java": `package com.example;
@Data
public class MyRequestDTO {
    private String name;
}`,
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	classes := []ClassDetail{
		{Name: "MyService", File: "MyService.java", Stereotype: "service", Dependencies: []string{"MyRepo"}},
		{Name: "MyRepo", File: "MyRepo.java", Stereotype: "repository"},
		{Name: "MyRequestDTO", File: "MyRequestDTO.java", Stereotype: ""},
	}

	excerpts := readFileSignatures(dir, classes, "")
	found := make(map[string]bool)
	for _, e := range excerpts {
		found[e.ClassName] = true
	}

	if !found["MyService"] {
		t.Error("missing seed MyService")
	}
	if !found["MyRepo"] {
		t.Error("missing BFS level 1 MyRepo")
	}
	// MyRequestDTO should be caught by name-based fallback (contains "dto")
	if !found["MyRequestDTO"] {
		t.Error("missing fallback DTO MyRequestDTO")
	}
}
