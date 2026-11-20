---
title: "Configuration"
description: Living specification of how a platform operator supplies the broker's settings and learns at startup when they are wrong. Every example on this page is checked against tests.
---

# Configuration

A platform operator gives the broker its settings before it serves a single request. This page
describes where the broker reads settings from, which source wins, and what the operator sees
when a setting is wrong. It describes the mechanism. The list of settings is in the
[configuration reference](../configuration.md).

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the test that
proves it. `just living-docs` runs the tests and fails when the page and the broker disagree.
The tests behind this page are plain Go tests, so the page shows no recorded HTTP exchanges. The
configuration snippets are illustrations copied from test fixtures and from `examples/config/`.
All values are synthetic.
:::

## The cast

An operator runs the broker in three environments. The settings that all environments share are
in a YAML file in version control. Each environment changes a few values with environment
variables. During an incident, the operator changes one value with a command-line flag and
restarts the broker.

The operator needs two guarantees. A value from a more specific source always wins. A wrong
value stops the broker at startup, with a message that names the setting.

## Four layers, and the more specific one wins

The broker reads each setting from four layers:

1. Built-in defaults.
2. The YAML file.
3. Environment variables. Their names start with `IDENTITY_BROKER_`.
4. Command-line flags.

A later layer overrides an earlier one, one setting at a time. `example:configuration.precedence.layers`

Take this YAML file (illustration, from the test fixture):

```yaml
log:
  level: debug
  format: text
server:
  enduser:
    port: 9000
    bind: "127.0.0.1"
    public_url: http://localhost:9000
```

and this environment and command line (illustration):

```bash
export IDENTITY_BROKER_SERVER_ENDUSER_PORT=9500
export IDENTITY_BROKER_LOG_LEVEL=warn
agentic-identity-broker --server.enduser.port=10000 --log-level=error
```

The test loads these settings four times and adds one layer each time:

| Layers present | End-user port | Bind address | Log level |
|---|---|---|---|
| Defaults only | `8000` | `::` | `info` |
| + YAML file | `9000` | `127.0.0.1` | `debug` |
| + environment variables | `9500` | `127.0.0.1` | `warn` |
| + flags | `10000` | `127.0.0.1` | `error` |

The bind address stays `127.0.0.1` in the last two rows. No environment variable and no flag
sets it, so the YAML value remains. A layer overrides only the settings that it names.

### "Off" and zero are values, not gaps

An override does not have to be a "larger" value. A YAML file can set a default-on switch to
`false` and a rate to `0`. An environment variable can set `false` and `0` over a YAML file that
says `true` and `0.5`. A flag can set `false` over an environment variable that says `true`. `example:configuration.precedence.false-and-zero`
The broker never treats `false` or `0` from a higher layer as "not set".

## The YAML file

The environment variable `IDENTITY_BROKER_CONFIG_PATH` selects the YAML file. The complete
example files `examples/config/config.development.yaml`, `examples/config/config.staging.yaml`,
and `examples/config/trust-anchor-local-jwks.yaml` load this way, and the broker records each as
a YAML source. `example:configuration.yaml.selected-by-environment`

The file is optional. When the selected path names no file, the broker loads its settings from
the other layers and starts. `example:configuration.yaml.optional`

### References to environment variables

A YAML value can refer to an environment variable with `${NAME}`. The reference can be the whole
value or a part of it (illustration, from the test fixture):

```yaml
telemetry:
  resource_attributes:
    service: broker
    service.namespace: ${BROKER_RESOURCE_NAMESPACE}
  exporter:
    headers:
      x.vendor.token: Bearer ${BROKER_EXPORTER_TOKEN}
```

With `BROKER_RESOURCE_NAMESPACE=production`, the namespace attribute becomes `production`, and
the header becomes `Bearer ` followed by the value of `BROKER_EXPORTER_TOKEN`. The values next to
them stay as written. `example:configuration.substitution.resolved`
This keeps a secret out of the file: the file names the variable, and the environment holds the
value.

A reference to a variable that is not set is an error. The broker does not replace it with an
empty text. The error names the variable: `environment variable 'TRUST_ANCHOR_TEST_UNSET' is not set`.
When the setting holds key material, the error does not repeat the value of the setting. `example:configuration.substitution.undefined`

## Files of environment variables

The broker also reads `.env` files from its working directory. A line such as
`IDENTITY_BROKER_LOG_LEVEL=debug` sets `log.level`, and
`IDENTITY_BROKER_SERVER_ENDUSER_PORT=8200` sets `server.enduser.port`. The broker records the
file as a source with the settings that it supplied. `example:configuration.env-file.loaded`

## Flags

The broker has flags for the settings that an operator changes most often at the command line,
for example `--log-level`, `--log-format`, `--server.enduser.port`, `--server.enduser.bind`,
`--server.admin.port`, `--server.admin.bind`, and `--server.shutdown.timeout`. The broker
records the flags as their own source and lists exactly the settings that the command line set. `example:configuration.flags.recorded-as-source`

## A wrong setting stops the broker at startup

The broker checks the complete configuration before it opens a port. An operator learns of a
mistake from the start command, not from the first failed request.

A YAML file that is not valid YAML stops the start. The error says that the configuration did
not load and that the file needs valid YAML syntax. `example:configuration.startup.malformed-yaml`

A value that is out of range also stops the start. With the end-user port set to `-1`, the error
names `server.enduser.port`. The admin port is still free afterwards: the broker did not start
one server and leave the other out. `example:configuration.startup.invalid-value`

The check also covers the form of a value. A duration needs a unit, such as `30s` or `500ms`. A
bare number is rejected, and the error names the setting and says that it expects a duration: `example:configuration.validation.duration-string`

```yaml
oauth2_authorization_server:
  mode: proxy
  proxy:
    upstream_timeout: 30   # rejected: write 30s
```

Some settings have no default, and the broker does not start without them. The page
"Authorization server modes" describes the rules for `oauth2_authorization_server`. The page
"Trust anchors" describes key sets. The page "Broker credentials at a third-party service"
describes credential files. The page "Deployment" describes how Helm values become this
configuration.

## Log level and format

Two settings control the broker's own log. `log.level` accepts `debug`, `info`, `warn`, and
`error`. Other values, such as `verbose`, `trace`, or an empty value, are rejected. `example:configuration.log.level`
`log.format` accepts `text` and `json`, and rejects other values, such as `xml`. `example:configuration.log.format`
Without a value from any layer, the level is `info`. `example:configuration.precedence.layers`

## What the broker reports about its configuration

For each source that it read, the broker keeps the type of the source, its path, and the settings
that it supplied. It writes the settings in the same dotted form that the YAML file and the
reference use, such as `log.level` and `server.enduser.port`. A key that contains a dot of its
own, such as the resource attribute `service.namespace`, stays readable in that list. `example:configuration.summary.yaml-keys`
At startup the broker prints a summary from this record, so the operator can see which source
supplied a value.

The summary must not leak a secret. The broker replaces a value with `***REDACTED***` when the
name of its setting starts with `IDENTITY_BROKER_` or contains `PASSWORD`, `SECRET`, `TOKEN`,
`KEY`, `CREDENTIAL`, or `AUTH`, in upper or lower case. It shows other values, such as a log
level or a port, as they are. `example:configuration.summary.redaction`

## Not yet on this page

No test backs the following parts of the capability, so the page does not state them as facts:

- The order of the four `.env` files (`.env`, `.env.local`, and the two files for the environment
  that `GO_ENV` names), and the position of `.env` files in the order of layers.
- The `--config` flag as the way to select the YAML file, and its priority over
  `IDENTITY_BROKER_CONFIG_PATH`.
- The printed startup summary and the JSON record of loaded sources. The tests cover the
  redaction rule and the recorded sources, not the printed text.
- The effect of `log.level` and `log.format` on the log output.
- Default values in a reference, nested references, the rejection of circular references, and
  the rejection of shell command patterns in a value.

The list of settings, their environment variables, and their defaults is in the
[configuration reference](../configuration.md).

## Sources

The behavior on this page was decided in `specs/002-flexible-configuration/spec.md` and
`adrs/002-configuration-libraries.md`. The operator reference is `docs/configuration.md`, with
examples in `examples/config/`. The tests are in `tests/integration/config_test.go`,
`internal/config/loader_test.go`, `internal/config/loader_trust_anchor_test.go`,
`internal/config/validator_test.go`, `internal/config/redactor_test.go`, and
`cmd/agentic-identity-broker/root_test.go`.
