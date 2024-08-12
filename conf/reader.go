package conf

import (
	"github.com/atlassian/jec/git"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

const (
	LocalSourceType = "local"
	GitSourceType   = "git"

	DefaultBaseUrl = "https://api.atlassian.com"
)

var readFileFromGitFunc = readFileFromGit
var readFileFromLocalFunc = readFileFromLocal

var defaultConfFilepath = filepath.Join("~", "jec", "config.json")

func Read() (*Configuration, error) {

	err := os.Setenv("JEC_CONF_SOURCE_TYPE", "local")
	if err != nil {
		return nil, err
	}
	err2 := os.Setenv("JEC_CONF_LOCAL_FILEPATH", "conf/config.json")
	if err2 != nil {
		return nil, err2
	}

	confSourceType := os.Getenv("JEC_CONF_SOURCE_TYPE")
	conf, err := readFileFromSource(strings.ToLower(confSourceType))
	if err != nil {
		return nil, err
	}

	if os.Getenv("JEC_API_KEY") != "" {
		conf.ApiKey = os.Getenv("JEC_API_KEY")
	}
	print("apiKey: ", conf.ApiKey)

	err = validate(conf)
	if err != nil {
		return nil, err
	}

	addHomeDirPrefixToActionMappings(conf.ActionMappings)
	chmodLocalActions(conf.ActionMappings, 0700)

	conf.addDefaultFlags()

	return conf, nil
}

func readFileFromSource(confSourceType string) (*Configuration, error) {

	switch confSourceType {
	case GitSourceType:
		url := os.Getenv("JEC_CONF_GIT_URL")
		privateKeyFilepath := os.Getenv("JEC_CONF_GIT_PRIVATE_KEY_FILEPATH")
		passphrase := os.Getenv("JEC_CONF_GIT_PASSPHRASE")
		confFilepath := os.Getenv("JEC_CONF_GIT_FILEPATH")

		if privateKeyFilepath != "" {
			privateKeyFilepath = addHomeDirPrefix(privateKeyFilepath)
		}

		if confFilepath == "" {
			return nil, errors.New("Git configuration filepath could not be empty.")
		}

		return readFileFromGitFunc(url, privateKeyFilepath, passphrase, confFilepath)
	case LocalSourceType:
		confFilepath := os.Getenv("JEC_CONF_LOCAL_FILEPATH")

		if len(confFilepath) <= 0 {
			confFilepath = addHomeDirPrefix(defaultConfFilepath)
		} else {
			confFilepath = addHomeDirPrefix(confFilepath)
		}

		return readFileFromLocalFunc(confFilepath)
	case "":
		return nil, errors.Errorf("JEC_CONF_SOURCE_TYPE should be set as \"local\" or \"git\".")
	default:
		return nil, errors.Errorf("Unknown configuration source type[%s], valid types are \"local\" and \"git\".", confSourceType)
	}
}

func (c *Configuration) addDefaultFlags() {
	c.GlobalArgs = append(
		[]string{
			"-apiKey", c.ApiKey,
			"-jsmUrl", c.BaseUrl,
			"-logLevel", strings.ToUpper(c.LogLevel),
		},
		c.GlobalArgs...,
	)
}

func validate(conf *Configuration) error {

	if conf == nil || conf == (&Configuration{}) {
		return errors.New("The configuration is empty.")
	}
	if conf.ApiKey == "" {
		return errors.New("ApiKey is not found in the configuration file.")
	}
	if conf.BaseUrl == "" {
		conf.BaseUrl = DefaultBaseUrl
		logrus.Infof("BaseUrl is not found in the configuration file, default url[%s] is set.", DefaultBaseUrl)
	}

	if len(conf.ActionMappings) == 0 {
		return errors.New("Action mappings configuration is not found in the configuration file.")
	} else {
		for actionName, action := range conf.ActionMappings {
			if action.SourceType != LocalSourceType &&
				action.SourceType != GitSourceType {
				return errors.Errorf("Action source type of action[%s] should be either local or git.", actionName)
			} else {
				if action.Filepath == "" {
					return errors.Errorf("Filepath of action[%s] is empty.", actionName)
				}
				if action.SourceType == GitSourceType &&
					action.GitOptions == (git.Options{}) {
					return errors.Errorf("Git options of action[%s] is empty.", actionName)
				}
			}
		}
	}

	level, err := logrus.ParseLevel(conf.LogLevel)
	if err != nil {
		conf.LogrusLevel = logrus.InfoLevel
		conf.LogLevel = "info"
	} else {
		conf.LogrusLevel = level
	}

	return nil
}
