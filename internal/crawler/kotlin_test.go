package crawler

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func TestKotlinParser_Basics(t *testing.T) {
	kp := NewKotlinParser()

	if kp.Platform() != model.Kotlin {
		t.Errorf("expected Kotlin platform, got %v", kp.Platform())
	}
	if exts := kp.Extensions(); len(exts) != 2 || exts[0] != ".kt" {
		t.Errorf("expected [.kt .kts], got %v", exts)
	}
}

func TestKotlinParser_ViewModel(t *testing.T) {
	src := []byte(`
package com.example.platform.commerce

import androidx.lifecycle.ViewModel
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

@HiltViewModel
class AppViewModel @Inject constructor(
    private val repository: AppRepository
) : ViewModel() {
    fun loadOffers(guestId: String): List<Offer> {
        return repository.getOffers(guestId)
    }
    suspend fun refreshProfile(accountId: String) {}
}
`)
	units, err := kp.ParseFile("app/src/main/kotlin/AppViewModel.kt", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	u := units[0]
	if u.Name != "AppViewModel" {
		t.Errorf("expected AppViewModel, got %s", u.Name)
	}
	if u.Stereotype != "viewmodel" {
		t.Errorf("expected viewmodel stereotype, got %s", u.Stereotype)
	}
	if len(u.Methods) < 2 {
		t.Errorf("expected at least 2 methods, got %d", len(u.Methods))
	}
	hasAppRepo := false
	for _, dep := range u.Dependencies {
		if dep == "AppRepository" {
			hasAppRepo = true
		}
	}
	if !hasAppRepo {
		t.Errorf("expected AppRepository in dependencies, got %v", u.Dependencies)
	}
}

var kp = NewKotlinParser()

func TestKotlinParser_RetrofitService(t *testing.T) {
	src := []byte(`
package com.example.platform.commerce.api

import retrofit2.http.GET
import retrofit2.http.POST

interface AppApiService {
    @GET("/api/app/offers/{guestId}")
    suspend fun getOffers(guestId: String): List<OfferDto>

    @POST("/api/app/preferences")
    suspend fun savePreferences(prefs: PreferencesDto): Response<Unit>
}
`)
	units, err := kp.ParseFile("api/AppApiService.kt", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	u := units[0]
	if u.Name != "AppApiService" {
		t.Errorf("expected AppApiService, got %s", u.Name)
	}
	if u.Stereotype != "api_service" {
		t.Errorf("expected api_service stereotype, got %s", u.Stereotype)
	}
	if len(u.Endpoints) != 2 {
		t.Errorf("expected 2 endpoints, got %d: %v", len(u.Endpoints), u.Endpoints)
	}
}

func TestKotlinParser_RoomEntity(t *testing.T) {
	src := []byte(`
package com.example.platform.commerce.db

import androidx.room.Entity
import androidx.room.Dao
import androidx.room.Query

@Entity(tableName = "offers")
data class OfferEntity(
    val id: String,
    val title: String
)

@Dao
interface OfferDao {
    @Query("SELECT * FROM offers")
    fun getAllOffers(): List<OfferEntity>
}
`)
	units, err := kp.ParseFile("db/OfferEntity.kt", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 {
		t.Fatalf("expected 2 units, got %d", len(units))
	}
	if units[0].Stereotype != "entity" {
		t.Errorf("expected entity stereotype, got %s", units[0].Stereotype)
	}
	if units[1].Stereotype != "repository" {
		t.Errorf("expected repository stereotype for DAO, got %s", units[1].Stereotype)
	}
}

func TestKotlinParser_Activity(t *testing.T) {
	src := []byte(`
package com.example.platform.commerce.ui

class AppMainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
    }
}
`)
	units, err := kp.ParseFile("ui/AppMainActivity.kt", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Stereotype != "activity" {
		t.Errorf("expected activity stereotype, got %s", units[0].Stereotype)
	}
}

func TestKotlinParser_Composable(t *testing.T) {
	src := []byte(`
package com.example.platform.commerce.ui

import androidx.compose.runtime.Composable

@Composable
fun AppScreen(viewModel: AppViewModel) {
}
`)
	units, err := kp.ParseFile("ui/AppScreen.kt", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Stereotype != "composable_screen" {
		t.Errorf("expected composable_screen stereotype, got %s", units[0].Stereotype)
	}
}

func TestKotlinParser_BuildConfig(t *testing.T) {
	src := []byte(`
package com.example.platform.commerce.config

class ApiConfig {
    val baseUrl = BuildConfig.API_BASE_URL
    val apiKey = BuildConfig.APP_API_KEY
}
`)
	units, err := kp.ParseFile("config/ApiConfig.kt", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if len(units[0].ConfigRefs) < 2 {
		t.Errorf("expected at least 2 config refs, got %d: %v", len(units[0].ConfigRefs), units[0].ConfigRefs)
	}
}
