FIXTURE := internal/app/testdata/mixed
DEMO    := demo
BIN     := bin/reconcile
RUN     := $(BIN) -hr $(DEMO)/hr.json -app $(DEMO)/app.json -policy policy.json

.PHONY: setup build plan apply failure-demo test clean

DEMO_FILES := $(DEMO)/hr.json $(DEMO)/app.json

define copy-fixture
	mkdir -p $(DEMO)
	cp $(FIXTURE)/hr.json $(FIXTURE)/app.json $(DEMO)/
endef

# Always resets the demo data to the fixture.
setup: build
	$(copy-fixture)

# Creates the demo data only when it is missing, so plan/apply keep the state
# left by earlier applies.
$(DEMO_FILES) &:
	$(copy-fixture)

build:
	go build -o $(BIN) ./cmd/app

plan: build $(DEMO_FILES)
	$(RUN)

apply: build $(DEMO_FILES)
	$(RUN) -apply

failure-demo: setup
	@echo "== 1. Apply, failing the first write for E004, its create (revocations before it are kept)"
	@$(RUN) -apply -fail-on=E004; status=$$?; \
		echo "== exit status: $$status"; test $$status -ne 0
	@echo "== 2. Apply again from the saved state: completes the remaining actions"
	@$(RUN) -apply
	@echo "== 3. Apply once more: no changes"
	@$(RUN) -apply

test:
	go test ./...

clean:
	rm -rf $(DEMO) bin
