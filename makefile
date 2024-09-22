.PHONY: generate-mock

generate-mock:
	mockgen -source=rdbms.go -destination=rdbms_mockgen.go -package=wsqlx