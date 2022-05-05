all:
	@echo "**********************************************************"
	@echo "**                    chi build tool                    **"
	@echo "**********************************************************"


test:
	go clean -testcache && $(MAKE) test-middleware

test-middleware:
	go test -race -v