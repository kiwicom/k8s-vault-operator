package controllers

import (
	"bufio"
	"encoding/json"
	"time"
)

type seedData struct {
	path string
	data map[string]any
}

type logEntry struct {
	Level           string    `json:"level"`
	TS              time.Time `json:"ts"`
	Msg             string    `json:"msg"`
	Controller      string    `json:"controller"`
	ControllerGroup string    `json:"controllerGroup"`
	ControllerKind  string    `json:"controllerKind"`
	VaultSecret     struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"VaultSecret"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	ReconcileID string `json:"reconcileID"`
	Error       string `json:"error"`
	Stacktrace  string `json:"stacktrace"`
}

func getLogsToVSName(name string) []logEntry {
	var entries []logEntry
	scanner := bufio.NewScanner(&logSink)
	for scanner.Scan() {
		var entry logEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil
		}
		if entry.VaultSecret.Name == name {
			entries = append(entries, entry)
		}
	}
	return entries
}

var (
	v1Data = []seedData{
		{
			path: "v1/something/a/secret",
			data: map[string]any{
				"a": "1",
			},
		},
		{
			path: "v1/something/a/secretx",
			data: map[string]any{
				"a": "x",
			},
		},
		{
			path: "v1/something/b/secret",
			data: map[string]any{
				"b": "2",
			},
		},
	}
	data = []seedData{
		{
			path: "secret/seeds/team1/project1/secret",
			data: map[string]any{
				"a": "1",
				"b": "10",
			},
		},
		{
			path: "secret/seeds/team1/project1/config",
			data: map[string]any{
				"a": "2",
			},
		},
		{
			path: "secret/seeds/team1/project1/apikey",
			data: map[string]any{
				"a": "super-secret-api-key",
			},
		},
		{
			path: "secret/seeds/team1/project2/secret",
			data: map[string]any{
				"a": "3",
				"b": "33",
			},
		},
		{
			path: "secret/seeds/team1/project2/config",
			data: map[string]any{
				"a": "4",
			},
		},
		{
			path: "secret/seeds/team2/project1/secret",
			data: map[string]any{
				"aa": "5",
			},
		},
		{
			path: "secret/seeds/team2/project1/config",
			data: map[string]any{
				"a": "6",
			},
		},
		{
			path: "secret/seeds/team2/project2/secret",
			data: map[string]any{
				"a": "7",
			},
		},
		{
			path: "secret/seeds/team2/project2/config",
			data: map[string]any{
				"a": "8",
			},
		},
		{
			path: "secret/seeds/team3/project1/json",
			data: map[string]any{
				"a/b": `{"a":"b"}`,
				"a":   `{"a":"b"}`,
			},
		},
		{
			path: "secret/seeds/team4/project1/secret",
			data: map[string]any{
				"mysecret": "myvalue",
			},
		},
		{
			path: "secret/seeds/team4/project2/complex",
			data: map[string]any{
				"a": "example1",
				"b": 123,
				"c": map[string]any{
					"c1": "example2",
					"c2": 456,
				},
				"d": []any{
					"first",
					"second",
					"third",
				},
			},
		},
	}
)
