# lotus-pilot
多个miner封存时，自动切换worker到指定miner
## 部署
编译
```bash
git clone https://github.com/gh-efforts/lotus-pilot.git
cd lotus-pilot
make
```
初始化
```bash
./lotus-pilot init

➜  .lotuspilot tree
.
├── config.json
├── scripts
│   ├── f017387.sh
│   └── f028064.sh
├── state
│   └── switch.json
└── template
    ├── worker32G.tmpl
    └── worker64G.tmpl
```
运行
```bash
./lotus-pilot run --listen 0.0.0.0:6788 --debug
```
## 配置
interval: 调用minerAPI 获取worker jobs状态的时间间隔，生产设置5m0s   
```json
{
	"interval": "1m0s",
	"miners": {
		"t017387": {
			"addr": "10.122.1.29:2345",
			"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.tlJ8d4RIudknLHrKDSjyKzfbh8hGp9Ez1FZszblQLAI"
		},
		"t028064": {
			"addr": "10.122.1.29:2346",
			"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.7ZoJAcyY9ictWUdWsiV5AwmSTPHCczkT8Y6mTiN3Azw"
		}
	}
}
```
## 功能
### miner manage
```bash
➜  lotus-pilot git:(main) ✗ ./lotus-pilot miner
NAME:
   lotus-pilot miner - manage miner list

USAGE:
   lotus-pilot miner command [command options] [arguments...]

COMMANDS:
   add      add new miner
   remove   remove miner
   list     list all miner
   worker   list miner workers
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --connect value  (default: "127.0.0.1:6788")
   --help, -h       show help
```
pilot 启动后会从 config 配置文件获取 miner 信息，并连接到 miner 。  
并根据 .lotuspilot/template 目录下的模版文件以及 miner 信息自动生成 worker 的启动脚本放在./lotuspilot/scripts目录下。  
启动后可以通过命令对miner进行增删改查，增加 miner 时会自动生成对应的 worker 启动脚本，同时更新 config 配置文件。   

### script manage
修改 .lotuspilot/template 目录下的模版文件    
为指定 miner 生成 worker 启动脚本：`lotus-pilot script create minerID`    
为所有 miner 生成 worker 启动脚本：`lotus-pilot script create all `  

### switch manage
```bash
➜  lotus-pilot git:(main) ✗ ./lotus-pilot switch -h
NAME:
   lotus-pilot switch - manage switch

USAGE:
   lotus-pilot switch command [command options] [arguments...]

COMMANDS:
   new      send new switch
   get      get switch state
   cancel
   remove
   list     get all switch id
   help, h  Shows a list of commands or help for one command
   ```
发起新的切换请求，设置不同的切换参数，以满足不同的切换场景。   
切换请求参数：
```golang
type SwitchRequest struct {
	From address.Address `json:"from"`
	To   address.Address `json:"to"`
	//如果Count为0，则切换所有worker
	Count int `json:"count"`
	//指定要切换的worker列表，如果为空，则由pilot选择
	Worker []uuid.UUID `json:"worker"`
	//切换前是否禁止AP任务，如果不禁止，则fromMiner的任务全部完成后再切到toMiner
	DisableAP bool `json:"disableAP"`
}
```
polit 接受请求后返回一个 switchID，可以根据 switchID 查看切换状态，取消，删除等。  
```bash
root@L01-W29:# ./lotus-pilot switch new --from f017387 --to f028064 --count 1 --disableAP                                                         
switchID: 03f93ab6-557e-4636-8364-255cc15fa15d                                                                                                                                  
worker:                                                                                                                                                                         
935fde10-7e51-47ef-ac7b-07bf9ca3e5ab DCZ-2007FD208U36-L06-W07 

root@L01-W29:# ./lotus-pilot switch get --switch-id 03f93ab6-557e-4636-8364-255cc15fa15d                                                          
switchID: 03f93ab6-557e-4636-8364-255cc15fa15d                                                                                                                                  
state: switching                                                                                                                                                                
switch request {From:f017387 To:f028064 Count:1 Worker:[] DisableAP:false}                                                                                                      
                                                                                                                                                                                
workerID: 935fde10-7e51-47ef-ac7b-07bf9ca3e5ab                                                                                                                                  
hostname: DCZ-2007FD208U36-L06-W07                                                                                                                                              
state: workerStoped                                                                                                                                                             
try: 0  
```
切换发起成功后（根据 switchID 查看状态）  
pilot 会定时（config interval）检查 worker 的状态，满足切换条件时进行切换，满足停止条件时则停止原 worker  

worker切换条件：
- sealing job 中这台 worker 没有 AP PC1 PC2 任务
- miner 调度队列中，这台 worker 没有 PC1 PC2任务  

worker stop 条件：
- sealing job 中这台 worker 没有任何任务
- miner索引中，这台 worker 没有 sector

切换状态会保存到: `.lotuspilot/state/switch.json`  
重启 pilot 会读取switch.json 恢复切换状态