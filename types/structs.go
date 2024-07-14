package types

type Config struct {
	Port                int    `json:"port"`
	Api_key             string `json:"api_key"`
	Version             string `json:"version"`
	UseBSON             bool   `json:"useBSON"`
	Cache_incoming_all  bool   `json:"cache_incoming_all"`
	Cache_incoming_time int    `json:"cache_incoming_time"`
	Cache_outgoing_all  bool   `json:"cache_outgoing_all"`
	Cache_outgoing_time int    `json:"cache_outgoing_time"`
}

type BinDataType int
type BinData struct {
	Name        string
	DataType    BinDataType
	DataContent []byte
}
