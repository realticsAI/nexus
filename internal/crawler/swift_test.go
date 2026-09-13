package crawler

import (
	"testing"
)

func TestSwiftParser_ParseFile(t *testing.T) {
	src := []byte(`
import UIKit
import Combine

class ProfileViewController: UIViewController {
    @IBOutlet var nameLabel: UILabel!
    @Published var userName: String = ""

    override func viewDidLoad() {
        super.viewDidLoad()
        loadProfile()
    }

    @IBAction func refreshTapped(_ sender: UIButton) {
        loadProfile()
    }

    func loadProfile() {
        let url = URL(string: "https://api.example.com/profile")!
        URLSession.shared.dataTask(with: url) { data, response, error in
        }.resume()
    }

    func readConfig() {
        let apiKey = Bundle.main.infoDictionary?["API_KEY"] as? String
    }
}

struct UserSettingsView: View {
    @State private var darkMode = false
    @EnvironmentObject var viewModel: SettingsViewModel

    var body: some View {
        Toggle("Dark Mode", isOn: $darkMode)
    }
}

class SettingsViewModel: ObservableObject {
    @Published var preferences: [String] = []

    func fetchPreferences() {
        AF.request("https://api.example.com/preferences").responseJSON { response in
        }
    }
}

struct UserProfile: Codable {
    let id: String
    let name: String
    let email: String
}

class NetworkService {
    func get(endpoint: String) -> AnyPublisher<Data, Error> {
        let subject = PassthroughSubject<Data, Error>()
        return subject.eraseToAnyPublisher()
    }
}
`)

	p := NewSwiftParser()
	units, err := p.ParseFile("Sources/Profile/ProfileViewController.swift", src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(units) == 0 {
		t.Fatal("expected units, got none")
	}

	byName := make(map[string]int)
	for i, u := range units {
		byName[u.Name] = i
	}

	// ProfileViewController
	if idx, ok := byName["ProfileViewController"]; ok {
		u := units[idx]
		if u.Stereotype != "viewcontroller" {
			t.Errorf("ProfileViewController: expected stereotype viewcontroller, got %q", u.Stereotype)
		}
		if u.Type != "class" {
			t.Errorf("ProfileViewController: expected type class, got %q", u.Type)
		}
		if len(u.Methods) < 3 {
			t.Errorf("ProfileViewController: expected at least 3 methods, got %d", len(u.Methods))
		}
		hasIBAction := false
		for _, m := range u.Methods {
			for _, a := range m.Annotations {
				if a == "@IBAction" {
					hasIBAction = true
				}
			}
		}
		if !hasIBAction {
			t.Error("ProfileViewController: expected @IBAction annotation on refreshTapped")
		}
	} else {
		t.Error("ProfileViewController not found")
	}

	// UserSettingsView (SwiftUI)
	if idx, ok := byName["UserSettingsView"]; ok {
		u := units[idx]
		if u.Stereotype != "swiftui_view" {
			t.Errorf("UserSettingsView: expected stereotype swiftui_view, got %q", u.Stereotype)
		}
		if u.Type != "struct" {
			t.Errorf("UserSettingsView: expected type struct, got %q", u.Type)
		}
	} else {
		t.Error("UserSettingsView not found")
	}

	// SettingsViewModel
	if idx, ok := byName["SettingsViewModel"]; ok {
		u := units[idx]
		if u.Stereotype != "viewmodel" {
			t.Errorf("SettingsViewModel: expected stereotype viewmodel, got %q", u.Stereotype)
		}
	} else {
		t.Error("SettingsViewModel not found")
	}

	// UserProfile (Codable model)
	if idx, ok := byName["UserProfile"]; ok {
		u := units[idx]
		if u.Stereotype != "model" {
			t.Errorf("UserProfile: expected stereotype model, got %q", u.Stereotype)
		}
	} else {
		t.Error("UserProfile not found")
	}

	// NetworkService
	if idx, ok := byName["NetworkService"]; ok {
		u := units[idx]
		if u.Stereotype != "api_service" {
			t.Errorf("NetworkService: expected stereotype api_service, got %q", u.Stereotype)
		}
	} else {
		t.Error("NetworkService not found")
	}

	t.Logf("Parsed %d units total", len(units))
	for _, u := range units {
		t.Logf("  [%s] %s (%s) — %d methods, %d annotations, %d api_calls",
			u.Stereotype, u.Name, u.Type, len(u.Methods), len(u.Annotations), len(u.ApiCalls))
	}
}
