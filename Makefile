all:
	@echo "**********************************************************"
	@echo "**                    chi build tool                    **"
	@echo "**********************************************************"


test:
	go clean -testcache && $(MAKE) test-router

test-router:
	go test -race -v .

.PHONY: docs
docs:
	npx docsify-cli serve ./docs
