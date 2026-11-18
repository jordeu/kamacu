package api

import (
	"fmt"
	"net/http"
	"testing"
)

func TestProjectPin(t *testing.T) {
	srv, db, _ := newTestServer(t)
	id := createProject(t, srv, gitRepo(t))
	url := fmt.Sprintf("%s/api/projects/%d", srv.URL, id)
	_, initial := doJSON(t, "GET", url, nil)
	if initial["pinned"] != false {
		t.Fatalf("new project pinned = %v, want false", initial["pinned"])
	}

	for _, pinned := range []bool{true, false} {
		status, body := doJSON(t, "PATCH", url, map[string]any{"pinned": pinned})
		if status != http.StatusOK || body["pinned"] != pinned {
			t.Fatalf("pin %v: status=%d body=%v", pinned, status, body)
		}
		var stored bool
		if err := db.QueryRow("SELECT pinned FROM projects WHERE id = ?", id).Scan(&stored); err != nil || stored != pinned {
			t.Fatalf("stored pin = %v, err=%v, want %v", stored, err, pinned)
		}
		// Other partial updates must preserve pin state.
		status, body = doJSON(t, "PATCH", url, map[string]any{"name": "Renamed"})
		if status != http.StatusOK || body["pinned"] != pinned {
			t.Fatalf("rename reset pin: status=%d body=%v", status, body)
		}
		_, listed := doJSONList(t, srv.URL+"/api/projects")
		if len(listed) != 1 || listed[0]["pinned"] != pinned {
			t.Fatalf("listed pin = %v, want %v", listed, pinned)
		}
		_, fetched := doJSON(t, "GET", url, nil)
		if fetched["pinned"] != pinned {
			t.Fatalf("fetched pin = %v, want %v", fetched["pinned"], pinned)
		}
	}
	status, _ := doJSON(t, "PATCH", url, map[string]any{"pinned": "true"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid pin status=%d, want 400", status)
	}
	_, unchanged := doJSON(t, "GET", url, nil)
	if unchanged["pinned"] != false {
		t.Fatal("invalid patch changed pin state")
	}
	status, _ = doJSON(t, "PATCH", srv.URL+"/api/projects/999999", map[string]any{"pinned": true})
	if status != http.StatusNotFound {
		t.Fatalf("unknown project status=%d, want 404", status)
	}
}
