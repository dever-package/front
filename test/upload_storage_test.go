package integration_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	frontmodel "github.com/dever-package/front/model"
	frontupload "github.com/dever-package/front/service/upload"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
	"github.com/jmoiron/sqlx"
	"github.com/shemic/dever/load"
	"github.com/shemic/dever/orm"
)

func TestUploadReuseFollowsConfiguredStorage(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.Mkdir("config", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("config/setting.json", []byte(`{"database":{"default":"default","persist":false,"create":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := orm.InitDefault(orm.Config{Driver: "sqlite3", DSN: filepath.Join(directory, "upload.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = orm.CloseAll() })
	for _, table := range []struct {
		name  string
		model any
	}{
		{"upload_file", frontmodel.UploadFile{}}, {"upload_storage", frontmodel.UploadStorage{}},
		{"upload_rule", frontmodel.UploadRule{}}, {"upload_accept_type", frontmodel.UploadAcceptType{}},
		{"upload_rule_accept_type", frontmodel.UploadRuleAcceptType{}}, {"upload_session", frontmodel.UploadSession{}},
		{"upload_file_biz", frontmodel.UploadFileBiz{}}, {"upload_file_cate", frontmodel.UploadFileCate{}},
	} {
		createUploadTestTable(t, db, table.name, table.model)
	}
	load.RegisterMany(map[string]any{
		"front.NewUploadFileModel": frontmodel.NewUploadFileModel, "front.NewUploadStorageModel": frontmodel.NewUploadStorageModel,
		"front.NewUploadRuleModel": frontmodel.NewUploadRuleModel, "front.NewUploadAcceptTypeModel": frontmodel.NewUploadAcceptTypeModel,
		"front.NewUploadRuleAcceptTypeModel": frontmodel.NewUploadRuleAcceptTypeModel, "front.NewUploadSessionModel": frontmodel.NewUploadSessionModel,
		"front.NewUploadFileBizModel": frontmodel.NewUploadFileBizModel, "front.NewUploadFileCateModel": frontmodel.NewUploadFileCateModel,
	})
	for _, statement := range []string{
		`CREATE UNIQUE INDEX upload_file_path ON upload_file(path)`,
		`INSERT INTO upload_storage(id,name,type,path_mode,domain) VALUES(1,'Original','local','hash','https://old.example.test'),(2,'Target','local','hash','https://new.example.test'),(3,'Failing target','qiniu','hash','https://failure.example.test')`,
		`INSERT INTO upload_rule(id,name,storage_id,accept_type_id,transport,max_size,status) VALUES(1,'Image',1,1,'relay',1,1)`,
		`INSERT INTO upload_accept_type(id,name,accept) VALUES(1,'PNG','image/png')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	input := frontupload.ImportFileInput{RuleID: 1, Kind: "image", Name: "same.png", Mime: "image/png", Content: buffer.Bytes()}
	ctx := context.Background()
	original, err := frontupload.ImportFile(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE upload_rule SET storage_id=3 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := frontupload.ImportFile(ctx, input); err == nil || !strings.Contains(err.Error(), "七牛云配置不完整") {
		t.Fatalf("cross-storage import reused the old file instead of reporting target failure: %v", err)
	}
	unchanged, err := uploadrepo.FindUploadFile(ctx, original.ID)
	if err != nil || unchanged.StorageID != 1 {
		t.Fatalf("failed upload changed original storage: %#v %v", unchanged, err)
	}
	if _, err := db.Exec("UPDATE upload_rule SET storage_id=2 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	stored, err := frontupload.ImportFile(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != original.ID || stored.StorageID != 2 || stored.Hash != original.Hash || stored.Path != original.Path {
		t.Fatalf("cross-storage import lost logical identity or retained old storage: %#v", stored)
	}
	if url := uploadrepo.ResolveUploadFileURLs(stored).URL; !strings.HasPrefix(url, "https://new.example.test/") {
		t.Fatalf("old storage URL returned: %s", url)
	}
	if _, err := db.Exec("UPDATE upload_storage SET type='qiniu' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	reused, err := frontupload.ImportFile(ctx, input)
	if err != nil || reused.ID != stored.ID || reused.StorageID != 2 {
		t.Fatalf("same-storage reuse attempted another transfer: %#v %v", reused, err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM upload_file"); err != nil || count != 1 {
		t.Fatalf("duplicate logical file records created: %d %v", count, err)
	}
}

func createUploadTestTable(t *testing.T, db *sqlx.DB, table string, model any) {
	t.Helper()
	snakeA := regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	snakeB := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	typ := reflect.TypeOf(model)
	var columns []string
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		name := strings.ToLower(snakeB.ReplaceAllString(snakeA.ReplaceAllString(field.Name, "${1}_${2}"), "${1}_${2}"))
		kind := "TEXT DEFAULT ''"
		if field.Type == reflect.TypeOf(time.Time{}) {
			kind = "DATETIME"
		} else if field.Type.Kind() >= reflect.Int && field.Type.Kind() <= reflect.Uint64 {
			kind = "INTEGER DEFAULT 0"
		}
		if name == "id" {
			kind = "INTEGER PRIMARY KEY"
		}
		columns = append(columns, name+" "+kind)
	}
	if _, err := db.Exec("CREATE TABLE " + table + " (" + strings.Join(columns, ",") + ")"); err != nil {
		t.Fatal(err)
	}
}
