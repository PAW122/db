package types

type Config struct {
	Port                int         `json:"port"`
	Tcp_Port            int         `json:"tcp_port"`
	Api_key             string      `json:"api_key"`
	Version             string      `json:"version"`
	UseBSON             bool        `json:"useBSON"`
	File_name           string      `json:"file_name"`
	Max_keys_per_file   int         `json:"max_keys_per_file"`
	Cache_incoming_all  bool        `json:"cache_incoming_all"`
	Cache_incoming_time int         `json:"cache_incoming_time"`
	Cache_outgoing_all  bool        `json:"cache_outgoing_all"`
	Cache_outgoing_time int         `json:"cache_outgoing_time"`
	Queue_save_size     int         `json:"queue_save_size"`
	Queue_delete_size   int         `json:"queue_delete_size"`
	Queue_read_size     int         `json:"queue_read_size"`
	Queue_add_size      int         `json:"queue_add_size"`
	Max_goroutines      int         `json:"max_goroutines"`
	Batch_Size          int         `json:"batch_size"`
	AsfsConfig          ASFS_config `json:"asfs_config"`
	AsqsConfig          ASQS_config `json:"asqs_config"`
}

type ASFS_config struct {
	Enable        bool `json:"enable"`
	Max_cpu_usage int  `json:"max_cpu_usage"`
}

type ASQS_config struct {
	Enable                  bool `json:"enable"`
	Interval                int  `json:"interval"`
	Queue_threshold         int  `json:"queue_threshold"`
	Worker_count_multiplier int  `json:"worker_count_multiplier"`
}

type BinDataType int
type BinData struct {
	Name        string
	DataType    BinDataType
	DataContent []byte
}
