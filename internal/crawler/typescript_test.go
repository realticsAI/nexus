package crawler

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func TestTypeScriptParserFunction(t *testing.T) {
	tp := NewTypeScriptParser()
	if tp.Platform() != model.TypeScript {
		t.Fatal("wrong platform")
	}

	src := []byte(`export function fetchOrders(userId: string): Promise<Order[]> {
    return fetch("/api/orders?user=" + userId).then(r => r.json());
}

export function createOrder(data: OrderInput): Promise<Order> {
    return fetch("/api/orders", { method: "POST", body: JSON.stringify(data) }).then(r => r.json());
}`)

	units, err := tp.ParseFile("api.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("expected at least 2 units, got %d", len(units))
	}
	if units[0].Name != "fetchOrders" {
		t.Fatalf("expected fetchOrders, got %s", units[0].Name)
	}
}

func TestTypeScriptParserClass(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export class OrderService {
    async getOrders(): Promise<Order[]> {
        const response = await axios.get("/api/orders");
        return response.data;
    }
}`)

	units, err := tp.ParseFile("OrderService.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	if units[0].Name != "OrderService" {
		t.Fatalf("expected OrderService, got %s", units[0].Name)
	}
}

func TestTypeScriptNavigationConfig(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`import { createStackNavigator } from '@react-navigation/stack';

const Stack = createStackNavigator()

export function AppNavigator() {
    return (
        <Stack.Screen name="Commerce" component={AppScreen} />
        <Stack.Screen name="Offers" component={OffersScreen} />
    )
}`)

	units, err := tp.ParseFile("navigation/AppNavigator.tsx", src)
	if err != nil {
		t.Fatal(err)
	}

	foundNavConfig := false
	for _, u := range units {
		if u.Stereotype == "navigation_config" {
			foundNavConfig = true
			if u.Name != "Stack" {
				t.Fatalf("expected navigator name 'Stack', got %s", u.Name)
			}
			if len(u.Endpoints) < 2 {
				t.Fatalf("expected at least 2 screen endpoints, got %d: %v", len(u.Endpoints), u.Endpoints)
			}
		}
	}
	if !foundNavConfig {
		t.Fatal("expected navigation_config stereotype")
	}
}

func TestTypeScriptScreenDetection(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function AppScreen({ navigation }) {
    const route = useRoute()
    return <View><Text>Commerce</Text></View>
}`)

	units, err := tp.ParseFile("screens/AppScreen.tsx", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	if units[0].Stereotype != "screen" {
		t.Fatalf("expected screen stereotype, got %s", units[0].Stereotype)
	}
}

func TestTypeScriptNavigationCalls(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function AppHub({ navigation }) {
    const goToOffers = () => navigation.navigate('OffersScreen', { id: 1 })
    const goBack = () => navigation.push('HomeScreen')
    return <View />
}`)

	units, err := tp.ParseFile("screens/AppHub.tsx", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	deps := units[0].Dependencies
	if len(deps) < 2 {
		t.Fatalf("expected at least 2 navigation dependencies, got %d: %v", len(deps), deps)
	}
	found := map[string]bool{}
	for _, d := range deps {
		found[d] = true
	}
	if !found["OffersScreen"] {
		t.Fatal("expected navigation dep 'OffersScreen'")
	}
	if !found["HomeScreen"] {
		t.Fatal("expected navigation dep 'HomeScreen'")
	}
}

func TestTypeScriptReduxSlice(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`const appSlice = createSlice({
    name: 'commerce',
    initialState,
    reducers: {
        setOffers: (state, action) => { state.offers = action.payload },
        clearOffers: (state) => { state.offers = [] },
    },
})`)

	units, err := tp.ParseFile("store/appSlice.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range units {
		if u.Stereotype == "redux_slice" {
			found = true
			if u.Name != "appSlice" {
				t.Fatalf("expected appSlice, got %s", u.Name)
			}
			if len(u.Annotations) == 0 || u.Annotations[0] != "slice:commerce" {
				t.Fatalf("expected slice:commerce annotation, got %v", u.Annotations)
			}
			if len(u.Methods) < 2 {
				t.Fatalf("expected at least 2 reducer methods, got %d", len(u.Methods))
			}
		}
	}
	if !found {
		t.Fatal("expected redux_slice stereotype")
	}
}

func TestTypeScriptContextProvider(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`const AppContext = createContext({ offers: [] })`)

	units, err := tp.ParseFile("context/AppContext.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range units {
		if u.Stereotype == "context_provider" {
			found = true
			if u.Name != "AppContext" {
				t.Fatalf("expected AppContext, got %s", u.Name)
			}
		}
	}
	if !found {
		t.Fatal("expected context_provider stereotype")
	}
}

func TestTypeScriptNativeModules(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function getBiometrics() {
    const result = NativeModules.BiometricAuth.authenticate()
    if (Platform.OS === 'ios') {
        return NativeModules.FaceID.check()
    }
    return result
}`)

	units, err := tp.ParseFile("native/biometrics.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	apiCalls := units[0].ApiCalls
	foundBiometric := false
	foundFaceID := false
	for _, call := range apiCalls {
		if call == "NATIVE://BiometricAuth" {
			foundBiometric = true
		}
		if call == "NATIVE://FaceID" {
			foundFaceID = true
		}
	}
	if !foundBiometric {
		t.Fatalf("expected NATIVE://BiometricAuth in api calls, got %v", apiCalls)
	}
	if !foundFaceID {
		t.Fatalf("expected NATIVE://FaceID in api calls, got %v", apiCalls)
	}
	foundPlatform := false
	for _, a := range units[0].Annotations {
		if a == "platform-specific" {
			foundPlatform = true
		}
	}
	if !foundPlatform {
		t.Fatal("expected platform-specific annotation")
	}
}

func TestTypeScriptGraphQL(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte("export function useAppData() {\n    return useQuery(gql`query GetAppOffers { offers { id name } }`)\n}")

	units, err := tp.ParseFile("hooks/useAppData.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	foundGql := false
	for _, call := range units[0].ApiCalls {
		if call == "GQL://GetAppOffers" {
			foundGql = true
		}
	}
	if !foundGql {
		t.Fatalf("expected GQL://GetAppOffers, got %v", units[0].ApiCalls)
	}
}

func TestTypeScriptApiHook(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`const useAppApi = () => {
    return { getOffers: () => fetch('/api/offers') }
}`)

	units, err := tp.ParseFile("hooks/useAppApi.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range units {
		if u.Stereotype == "api_hook" {
			found = true
			if u.Name != "useAppApi" {
				t.Fatalf("expected useAppApi, got %s", u.Name)
			}
		}
	}
	if !found {
		t.Fatal("expected api_hook stereotype")
	}
}

func TestTypeScriptDeepAPIURLExtraction(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`const api = axios.create({ baseURL: 'https://api.example.com/api/v2' })

const BASE_URL = "/api/v1"

export function getCart(id: string) {
    return fetch('/api/cart/' + id)
}

export function getOrders() {
    return axios.get('/api/orders/history/recent')
}`)

	units, err := tp.ParseFile("services/api.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	calls := units[0].ApiCalls
	found := map[string]bool{}
	for _, c := range calls {
		found[c] = true
	}
	if !found["https://api.example.com/api/v2"] {
		t.Fatalf("expected axios.create baseURL, got %v", calls)
	}
	if !found["/api/v1"] {
		t.Fatalf("expected BASE_URL constant, got %v", calls)
	}
}

func TestTypeScriptTemplateLiteralCollapse(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte("export function getItem(id: string) {\n    return fetch(`${API_BASE}/cart/${id}/items`)\n}")

	units, err := tp.ParseFile("services/cart.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	calls := units[0].ApiCalls
	foundCollapsed := false
	for _, c := range calls {
		if c == "/cart/{id}/items" {
			foundCollapsed = true
		}
	}
	if !foundCollapsed {
		t.Fatalf("expected collapsed template literal /cart/{id}/items, got %v", calls)
	}
}

func TestTypeScriptApolloURI(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function createClient() {
    const client = new ApolloClient({
        uri: 'https://graph.loyalty.com/graphql',
        cache: new InMemoryCache()
    })
    return client
}`)

	units, err := tp.ParseFile("graphql/client.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	calls := units[0].ApiCalls
	found := false
	for _, c := range calls {
		if c == "https://graph.loyalty.com/graphql" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Apollo URI, got %v", calls)
	}
}

func TestTypeScriptTestFileExclusion(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function testHelper() {
    return fetch('/api/test-endpoint')
}`)

	units, err := tp.ParseFile("__tests__/api.test.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	if len(units[0].ApiCalls) != 0 {
		t.Fatalf("expected no API calls for test file, got %v", units[0].ApiCalls)
	}
	if !units[0].IsTest {
		t.Fatal("expected IsTest=true for test file")
	}
}

func TestCollapseTemplateLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"${API_BASE}/cart/${id}/items", "/cart/{id}/items"},
		{"/api/orders", "/api/orders"},
		{"${BASE}/users/${userId}/profile", "/users/{id}/profile"},
		{"${x}", ""},
	}
	for _, tt := range tests {
		got := collapseTemplateLiteral(tt.input)
		if got != tt.want {
			t.Errorf("collapseTemplateLiteral(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/components/Button.tsx", false},
		{"src/components/Button.test.tsx", true},
		{"src/components/Button.spec.ts", true},
		{"src/__tests__/Button.tsx", true},
		{"src/__mocks__/api.ts", true},
		{"test/setup.ts", true},
		{"tests/helpers.ts", true},
		{"hooks/useAppApi.ts", false},
		{"screens/HomeScreen.tsx", false},
	}
	for _, tt := range tests {
		got := isTestFile(tt.path)
		if got != tt.want {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestTestFileExcludesApiSignals(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function testFetchOrders() {
    const res = fetch("/api/orders")
    const data = axios.get("/api/products")
    const gqlRes = useQuery(gql` + "`" + `query GetOrders { orders { id } }` + "`" + `)
    const native = NativeModules.TestModule.call()
}`)

	units, err := tp.ParseFile("src/__tests__/api.test.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	if !units[0].IsTest {
		t.Fatal("expected IsTest=true for test file")
	}
	if len(units[0].ApiCalls) != 0 {
		t.Fatalf("expected no api calls from test file, got %v", units[0].ApiCalls)
	}
}

func TestNonTestFileKeepsApiSignals(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function fetchOrders() {
    return fetch("/api/orders").then(r => r.json())
}`)

	units, err := tp.ParseFile("src/api/orders.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	if units[0].IsTest {
		t.Fatal("expected IsTest=false for non-test file")
	}
	if len(units[0].ApiCalls) == 0 {
		t.Fatal("expected api calls from non-test file")
	}
}

func TestTypeScriptEnvVarCapture(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function getConfig() {
    const apiUrl = process.env.REACT_APP_API_URL
    const key = process.env.REACT_APP_API_KEY
    const nodeEnv = process.env.NODE_ENV
    return { apiUrl, key, nodeEnv }
}`)

	units, err := tp.ParseFile("config/env.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	refs := units[0].ConfigRefs
	found := map[string]bool{}
	for _, r := range refs {
		found[r] = true
	}
	if !found["env:REACT_APP_API_URL"] {
		t.Fatalf("expected env:REACT_APP_API_URL, got %v", refs)
	}
	if !found["env:REACT_APP_API_KEY"] {
		t.Fatalf("expected env:REACT_APP_API_KEY, got %v", refs)
	}
	if found["env:NODE_ENV"] {
		t.Fatalf("NODE_ENV should be filtered as noise, got %v", refs)
	}
}

func TestTypeScriptImportMetaEnv(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function getApiBase() {
    return import.meta.env.VITE_API_BASE + '/v2'
}`)

	units, err := tp.ParseFile("config/vite-env.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	refs := units[0].ConfigRefs
	found := false
	for _, r := range refs {
		if r == "env:VITE_API_BASE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected env:VITE_API_BASE in config refs, got %v", refs)
	}
}

func TestTypeScriptEnvVarDedup(t *testing.T) {
	tp := NewTypeScriptParser()
	src := []byte(`export function getStuff() {
    const a = process.env.REACT_APP_TOKEN
    const b = process.env.REACT_APP_TOKEN
    const c = import.meta.env.REACT_APP_TOKEN
    return a + b + c
}`)

	units, err := tp.ParseFile("config/dedup.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}

	count := 0
	for _, r := range units[0].ConfigRefs {
		if r == "env:REACT_APP_TOKEN" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 env:REACT_APP_TOKEN (deduped), got %d in %v", count, units[0].ConfigRefs)
	}
}

