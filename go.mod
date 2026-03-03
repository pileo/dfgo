module dfgo

go 1.23.0

require (
	github.com/google/uuid v1.6.0
	github.com/spf13/cobra v1.10.2
	github.com/strongdm/ai-cxdb/clients/go v0.0.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.0.12 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
)

replace github.com/strongdm/ai-cxdb/clients/go => ../cxdb/clients/go
