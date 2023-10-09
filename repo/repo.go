package repo

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"text/template"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/gh-efforts/lotus-pilot/repo/config"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
)

const (
	fsConfig    = "config.json"
	fsScripts   = "scripts"
	fsTemplate  = "template"
	fsState     = "state"
	fsWorker32G = "worker32G.tmpl"
	fsWorker64G = "worker64G.tmpl"
	fsSwitch    = "switch.json"
)

var log = logging.Logger("pilot/repo")

//go:embed template/*
var tmplFS embed.FS

type Repo struct {
	path       string
	configPath string
}

type MinerParse struct {
	MinerID      string
	MinerAPIInfo string
}

func New(path string) (*Repo, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, err
	}

	return &Repo{
		path:       path,
		configPath: filepath.Join(path, fsConfig),
	}, nil
}

func (r *Repo) Exists() (bool, error) {
	_, err := os.Stat(filepath.Join(r.path, fsScripts))
	notexist := os.IsNotExist(err)
	if notexist {
		err = nil

		_, err = os.Stat(filepath.Join(r.path, fsTemplate))
		notexist = os.IsNotExist(err)
		if notexist {
			err = nil
		}
	}
	return !notexist, err
}

func (r *Repo) Init() error {
	exist, err := r.Exists()
	if err != nil {
		return err
	}
	if exist {
		return nil
	}

	log.Infof("Initializing repo at '%s'", r.path)
	err = os.MkdirAll(r.path, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	err = os.MkdirAll(filepath.Join(r.path, fsScripts), 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	if err := r.initConfig(); err != nil {
		return err
	}

	if err := r.initTemplate(); err != nil {
		return err
	}

	if err := r.initState(); err != nil {
		return err
	}

	return nil

}

func (r *Repo) initConfig() error {
	data, err := json.MarshalIndent(config.DefaultConfig(), "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(r.configPath, data, 0666)
}

func (r *Repo) initTemplate() error {
	err := os.MkdirAll(filepath.Join(r.path, fsTemplate), 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	data, err := tmplFS.ReadFile(filepath.Join("template", fsWorker32G))
	if err != nil {
		return err
	}
	err = os.WriteFile(r.worker32G(), data, 0666)
	if err != nil {
		return err
	}

	data, err = tmplFS.ReadFile(filepath.Join("template", fsWorker64G))
	if err != nil {
		return err
	}

	return os.WriteFile(r.worker64G(), data, 0666)
}

func (r *Repo) initState() error {
	err := os.MkdirAll(filepath.Join(r.path, fsState), 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}

	return os.WriteFile(r.switchStateFile(), []byte("{}"), 0666)
}

func (r *Repo) LoadConfig() (*config.Config, error) {
	return config.LoadConfig(r.configPath)
}

func (r *Repo) worker32G() string {
	return filepath.Join(r.path, fsTemplate, fsWorker32G)
}

func (r *Repo) worker64G() string {
	return filepath.Join(r.path, fsTemplate, fsWorker64G)
}

func (r *Repo) ScriptsPath() string {
	return filepath.Join(r.path, fsScripts)
}

func (r *Repo) CreateScript(miner, token string, size abi.SectorSize) error {
	var t *template.Template
	var err error

	mp := MinerParse{
		MinerID:      miner,
		MinerAPIInfo: token,
	}

	if size == 68719476736 {
		t, err = template.ParseFiles(r.worker64G())
		if err != nil {
			return err
		}
	} else {
		t, err = template.ParseFiles(r.worker32G())
		if err != nil {
			return err
		}
	}

	name := filepath.Join(r.ScriptsPath(), miner+".sh")
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()

	err = t.Execute(f, mp)
	if err != nil {
		return err
	}

	log.Infof("create script: %s", name)
	return nil
}

func (r *Repo) RemoveScript(miner string) error {
	name := filepath.Join(r.ScriptsPath(), miner+".sh")
	err := os.Remove(name)
	if err != nil {
		return err
	}

	log.Infof("remove script: %s", name)
	return nil
}

// TODO: support concurrency
func (r *Repo) UpdateConfig(miner string, api config.APIInfo) error {
	c, err := config.LoadConfig(r.configPath)
	if err != nil {
		return err
	}

	if api == (config.APIInfo{}) {
		delete(c.Miners, miner)
	} else {
		c.Miners[miner] = api
	}

	data, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return err
	}

	err = os.WriteFile(r.configPath, data, 0666)
	if err != nil {
		return err
	}
	log.Infof("update config: %s", miner)
	return nil
}

func (r *Repo) switchStateFile() string {
	return filepath.Join(r.path, fsState, fsSwitch)
}

func (r *Repo) WriteSwitchState(data []byte) error {
	return os.WriteFile(r.switchStateFile(), data, 0666)
}

func (r *Repo) ReadSwitchState() ([]byte, error) {
	return os.ReadFile(r.switchStateFile())
}
