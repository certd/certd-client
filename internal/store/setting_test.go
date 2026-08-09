package store

import (
	"testing"

	"github.com/certd/certd-client/internal/store/repo"
)

func TestSettingIsStoredByKeyAsJSON(t *testing.T) {
	db, err := OpenDatabase("file:settings?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	repository := repo.NewSettingsRepository(db)

	if err := repository.SaveSetting("certd", `{"baseUrl":"https://certd.example.com","keyId":"key-1","keySecret":"secret-1"}`); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveSetting("sync", `{"retrySeconds":10}`); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveSetting("certd", `{"baseUrl":"https://certd.example.com","keyId":"key-2","keySecret":"secret-2"}`); err != nil {
		t.Fatal(err)
	}

	setting, err := repository.GetSetting("certd")
	if err != nil {
		t.Fatal(err)
	}
	if setting != `{"baseUrl":"https://certd.example.com","keyId":"key-2","keySecret":"secret-2"}` {
		t.Fatalf("unexpected Certd setting: %s", setting)
	}
	syncSetting, err := repository.GetSetting("sync")
	if err != nil {
		t.Fatal(err)
	}
	if syncSetting != `{"retrySeconds":10}` {
		t.Fatalf("unexpected sync setting: %s", syncSetting)
	}
}
