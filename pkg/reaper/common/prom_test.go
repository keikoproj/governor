package common

import (
<<<<<<< HEAD
=======
	"context"
>>>>>>> 7a298b8 (Pushgateway API)
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIs(t *testing.T) {

	// Fake a Pushgateway that responds with 202 to DELETE and with 200 in
	// all other cases.
	pgwOK := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			_, err = ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", `text/plain; charset=utf-8`)
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			w.WriteHeader(http.StatusOK)
		}),
	)
	defer pgwOK.Close()

	api := PrometheusAPI{
		Pushgateway: pgwOK.URL,
	}

	var tags = make(map[string]string)
	tags["app"] = "test"
	tags["namespace"] = "test-ns"

<<<<<<< HEAD
	err := api.SetMetricValue("abc", tags, 50)
=======
	err := api.SetMetricValue(context.Background(), "abc", tags, 50)
>>>>>>> 7a298b8 (Pushgateway API)
	assert.Nil(t, err)
}
