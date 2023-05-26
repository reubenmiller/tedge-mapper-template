# Topics

## firmware operation (c8y-firmware-plugin)

### Topics used to start the firmware update and the public status afterwards

```sh
tedge/child01/commands/firmware_update/start
tedge/child01/commands/firmware_update/executing
tedge/child01/commands/firmware_update/done/successful
tedge/child01/commands/firmware_update/done/failed
```

### Child device topics

These topics should only be used to communicate information between the c8y-firmware-plugin and the child device connector.

```sh
tedge/child01/commands/req/firmware_update
tedge/child01/commands/res/firmware_update
```
