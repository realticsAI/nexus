package crawler

import (
	"testing"

	"github.com/anurag/nexus/model"
)

func TestPythonParserClass(t *testing.T) {
	pp := NewPythonParser()
	if pp.Platform() != model.Python {
		t.Fatal("wrong platform")
	}

	src := []byte(`class OrderService:
    def __init__(self, db):
        self.db = db

    def get_order(self, order_id: str):
        return self.db.find(order_id)

    def create_order(self, data: dict):
        return self.db.insert(data)
`)

	units, err := pp.ParseFile("service.py", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("expected 1 unit, got %d", len(units))
	}
	if units[0].Name != "OrderService" {
		t.Fatalf("expected OrderService, got %s", units[0].Name)
	}
	if len(units[0].Methods) < 2 {
		t.Fatalf("expected at least 2 methods, got %d", len(units[0].Methods))
	}
}

func TestPythonParserDecorated(t *testing.T) {
	pp := NewPythonParser()
	src := []byte(`from flask import Flask

app = Flask(__name__)

@app.route("/api/orders")
def list_orders():
    return get_all_orders()

@app.route("/api/orders", methods=["POST"])
def create_order():
    return save_order(request.json)
`)

	units, err := pp.ParseFile("routes.py", src)
	if err != nil {
		t.Fatal(err)
	}
	hasEndpoint := false
	for _, u := range units {
		if len(u.Endpoints) > 0 {
			hasEndpoint = true
		}
	}
	if !hasEndpoint {
		t.Fatal("expected at least one unit with endpoints")
	}
}

func TestPythonParserFunction(t *testing.T) {
	pp := NewPythonParser()
	src := []byte(`def process_payment(amount: float, currency: str = "USD"):
    response = requests.post("https://payment.api/charge", json={"amount": amount})
    return response.json()
`)

	units, err := pp.ParseFile("payment.py", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) == 0 {
		t.Fatal("expected at least 1 unit")
	}
	if units[0].Name != "process_payment" {
		t.Fatalf("expected process_payment, got %s", units[0].Name)
	}
}
