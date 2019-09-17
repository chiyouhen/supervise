supervise:
	rm -Rf $(PWD)/output
	mkdir $(PWD)/output
	cd $(PWD)/output && GO111MODULE=on go build $(PWD)


