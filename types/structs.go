package types

type Config struct {
	Port    int    `json:"port"`
	Api_key string `json:"api_key"`
	Version string `json:"version"`
}

type BinDataType int
type BinData struct {
	Name        string
	DataType    BinDataType
	DataContent []byte
}
