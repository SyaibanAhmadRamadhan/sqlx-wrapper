.PHONY: generate-mock

generate-mock:
	mockgen -source=db_tx.go -destination=db_tx_mockgen.go -package=wsqlx