# lotus-pilot
多个miner封存时，自动切换worker到指定miner
## 部署
```bash
git clone https://github.com/gh-efforts/lotus-pilot.git
cd lotus-pilot
make
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
   help, h  Shows a list of commands or help for one command

OPTIONS:
   --connect value  (default: "127.0.0.1:6788")
   --help, -h       show help
```
pilot 启动后会从 config 配置文件获取 miner 信息，并连接到 miner 。  
并根据 template 目录下的模版文件以及 miner 信息自动生成 worker 的启动脚本放在scripts目录下。  
启动后可以通过命令对miner进行增删改查，增加 miner 时会自动生成对应的 worker 启动脚本。  

### script manage
修改 template 目录下的模版文件    
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