# JIRA EDGE CONNECTOR

[![Atlassian license](https://img.shields.io/badge/license-Apache%202.0-blue.svg?style=flat-square)](LICENSE)
[![Build Status](https://github.com/atlassian/jec/workflows/test/badge.svg?branch=master)](https://github.com/atlassian/jec/actions?query=workflow%3Atest)
[![Coverage Status](https://coveralls.io/repos/github/atlassian/jec/badge.svg?branch=master)](https://coveralls.io/github/atlassian/jec?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/atlassian/jec)](https://goreportcard.com/report/github.com/atlassian/jec)
[![GoDoc](https://godoc.org/github.com/atlassian/jec?status.svg)](https://godoc.org/github.com/atlassian/jec)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg?style=flat-square)](CONTRIBUTING.md)


Jira Edge Connector (JEC) is a lightweight application that provides:

* Jira Service Management integration for systems that don't need the inbound internet
* Ability to run executables and scripts triggered by Jira Service Management
* Deployment on-premises or in the customer’s cloud environment

JEC integrates with a number of monitoring and ITSM tools, allowing Jira Service Management to send actions back to keep various toolsets in sync across the organization. JEC also hosts custom scripts that can be executed remotely.

#### Supported Script Technologies

JEC includes support for running Groovy, Python and Go scripts, along with any .sh shell script or executable.

JEC supports environment variables, arguments, and flags that are passed to scripts. These can be set globally for all scripts or locally on a per script basis. Stderr and stdout options are also available.

#### Support for Git

JEC provides the ability to retrieve files from Git.

Configuration files for JEC can be maintained in Git to ensure version control. Likewise, scripts and credentials can be kept in Git and retrieved when needed so that credentials are not stored locally.

## Installation

### Environment Variables

#### Prerequisites

For setting configuration file properties such as location and path:

* First, you should set some environment variables for the locate configuration file.
  There are two options here, you can get the configuration file from a local drive or by using git.

For reading configuration files from a local drive:

* Set `JEC_CONF_SOURCE_TYPE` and `JEC_CONF_LOCAL_FILEPATH` variables.

From reading configuration files from a git repository:

* Set `JEC_CONF_SOURCE_TYPE`, `JEC_CONF_GIT_URL`, `JEC_CONF_GIT_FILEPATH`, `JEC_CONF_GIT_PRIVATE_KEY_FILEPATH`, and `JEC_CONF_GIT_PASSPHRASE` variables.

```If you are using a public repository, you should use an https format of a git url and you do not need to set private key and passphrase.```

For more information, you can visit [JEC documentation page]() // TODO: Add link
### Flag
Prometheus default metrics can be grabbed from `http://localhost:<port-number>/metrics`

To run multiple JEC in the same environment, -jec-metrics flag should be set as distinct port number values.
`-jec-metrics <port-number>`

### Logs
JEC log file is located:

* On Windows: `var/log/jec/jec<pid>.log`
* On Linux: `/var/log/jec/jec<pid>.log`
* At the end of the file name of the log, there is program identifier (pid) to identify which process is running.

### Configuration File
JEC supports json and yaml file extension with fields.

For definition of all fields which should be provided in configuration file, you can visit [JEC documentation page]() // TODO: Add link

## Usage

You can run executable that you build according the building JEC executables section.
```
JEC_CONF_SOURCE_TYPE=LOCAL JEC_CONF_LOCAL_FILEPATH=$JEC_FILE_PATH ./main
```
Also you can run JEC by using Docker. For more information, please visit [documentation]() // TODO: Add link

## Tests

You can run tests with `go test`

## Contributions

Contributions to JEC are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

Copyright (c) 2023 Atlassian US., Inc.
Apache 2.0 licensed, see [LICENSE](LICENSE) file.

<br/>

[![With â¤ï¸ from Atlassian](https://raw.githubusercontent.com/atlassian-internal/oss-assets/master/banner-with-thanks-light.png)](https://www.atlassian.com)