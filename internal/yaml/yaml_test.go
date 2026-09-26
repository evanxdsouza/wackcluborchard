package yaml

import (
	"encoding/json"
	"testing"
)

func TestCompose(t *testing.T) {
	src := `
version: "3.9"
services:
  web:
    image: nginx:alpine   # comment
    ports:
      - "8080:80"
    environment:
      - FOO=bar
      - "BAZ=qux # not a comment"
    depends_on: [db, cache]
  db:
    image: postgres:17
    environment:
      POSTGRES_PASSWORD: secret
      EMPTY:
    command: >
      postgres
      -c max_connections=200
    healthcheck:
      test: ["CMD", "pg_isready"]
  worker:
    build:
      context: .
      dockerfile: Dockerfile
    deploy: {replicas: 2, resources: {limits: {cpus: "0.5"}}}
list:
- a
- b: 1
  c: 2
-
  - nested
script: |
  echo hi
  echo there
`
	v, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	want := `{"list":["a",{"b":1,"c":2},["nested"]],"script":"echo hi\necho there\n","services":{"db":{"command":"postgres -c max_connections=200\n","environment":{"EMPTY":null,"POSTGRES_PASSWORD":"secret"},"healthcheck":{"test":["CMD","pg_isready"]},"image":"postgres:17"},"web":{"depends_on":["db","cache"],"environment":["FOO=bar","BAZ=qux # not a comment"],"image":"nginx:alpine","ports":["8080:80"]},"worker":{"build":{"context":".","dockerfile":"Dockerfile"},"deploy":{"replicas":2,"resources":{"limits":{"cpus":"0.5"}}}}},"version":"3.9"}`
	if string(b) != want {
		t.Fatalf("got\n%s\nwant\n%s", b, want)
	}
}

func TestKubeconfig(t *testing.T) {
	src := `apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: AAAA
    server: https://127.0.0.1:6443
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
kind: Config
users:
- name: default
  user:
    token: abc
`
	v, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	want := `{"apiVersion":"v1","clusters":[{"cluster":{"certificate-authority-data":"AAAA","server":"https://127.0.0.1:6443"},"name":"default"}],"contexts":[{"context":{"cluster":"default","user":"default"},"name":"default"}],"current-context":"default","kind":"Config","users":[{"name":"default","user":{"token":"abc"}}]}`
	if string(b) != want {
		t.Fatalf("got\n%s\nwant\n%s", b, want)
	}
}
