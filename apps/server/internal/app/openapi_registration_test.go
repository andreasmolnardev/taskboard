package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/pocketbase/pocketbase"
)

func TestRegisterMakesTodoOperationAvailableBeforeServe(t *testing.T) {
	pb := pocketbase.New()
	api := humago.New(http.NewServeMux(), huma.DefaultConfig("Slopstack API", "0.1.0"))

	if err := Register(pb, Config{HTTPAddress: ":8090", PublicURL: "http://localhost:8090"}, api); err != nil {
		t.Fatal(err)
	}

	document, err := json.Marshal(api.OpenAPI())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document), `"/api/todos"`) {
		t.Fatal("OpenAPI document does not contain /api/todos before server start")
	}
	for _, operationID := range []string{"listTodos", "createTodo", "getTodo", "updateTodo", "deleteTodo"} {
		if !strings.Contains(string(document), `"operationId":"`+operationID+`"`) {
			t.Errorf("OpenAPI document does not contain operation %q", operationID)
		}
	}
}
