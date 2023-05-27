# Examples

A collection of examples which are provided by the in-built routes.

## Child device registration

Child devices can be registered to the main device or other child devices. This level is currently limited to children of children (2 level device hierarchy, however this will likely be lifted in the future).

Child devices can be registered with some additional information which let's thin-edge.io know what functionality is supported by the device. An example of the the payload used to register a child device is shown below.

```json
{
    "requiredInterval": 10,
    "supportedOperations": [
        "c8y_Restart",
        "c8y_SoftwareUpdate"
    ],
}
```

|Property|Description|Example|
|--|--|--|
|`requiredInterval`|Required interval in minutes.|`30`|
|`supportedOperations`|List of operations which the child device supports|`["c8y_Restart"]`|

### Register a child device

**Topic**

A child device can be registered to the main device using the following topic.

```sh
tedge/register/child/{child}
```

**Example**

```sh
tedge mqtt pub tedge/register/child/child10 '{"supportedOperations":["c8y_Restart", "c8y_SoftwareUpdate"]}'
```

### Register a child device of an existing child device

```sh
tedge mqtt pub tedge/child10/register/child/child10_01 '{"supportedOperations":["c8y_Restart", "c8y_SoftwareUpdate"]}'
```

tedge mqtt pub tedge/child10/register/child/child10_03 '{"supportedOperations":["c8y_Restart", "c8y_SoftwareUpdate"],"requiredInterval":10}'

## Inventory updates

### Inventory updates

Updating multiple properties on the root level or non-object updates (like strings, numbers or boolean).

```sh
tedge mqtt pub "tedge/inventory/update" '{"type":"mytype", "custom":{"os":"Debian 11"}}'
tedge mqtt pub "tedge/{child}/inventory/update" '{"type":"mytype", "custom":{"os":"Debian 11"}}'
```

**Main Device**

```sh
tedge mqtt pub tedge/inventory/update '{"custom":{"os":"Debian 11"}}'
```

**Child device**

```sh
tedge mqtt pub tedge/child01/inventory/update '{"custom":{"os":"Debian 11"}}'
```

### Partial inventory updates

Properties on the root level can also be updated by using the topic structure (currently only the root level is supported). This is the preferred method to update fragments as it allows other components to listen to a subset of changes, rather than every inventory update.

```sh
tedge mqtt pub "tedge/inventory/update/{fragment}" '{"os":"Debian 11"}'
tedge mqtt pub "tedge/{child}/inventory/update/{fragment}" '{"os":"Debian 11"}'
```

The fragment in the topic will be used to place the payload like so:

```json
{
    "{fragment}": {
        "os": "Debian 11"
    }
}
```

**Main Device**

```sh
tedge mqtt pub tedge/inventory/update/custom '{"os":"Debian 12"}'
```

**Child device**

```sh
tedge mqtt pub tedge/child01/inventory/update/custom '{"os":"Debian 12"}'
```

### Deleting a fragment

Single fragments can be deleted using the following topics.

```sh
tedge mqtt pub "tedge/inventory/delete/{fragment}" ''
tedge mqtt pub "tedge/{child}/inventory/delete/{fragment}" ''
```

**Main Device**

```sh
tedge mqtt pub tedge/inventory/delete/custom ''
```

**Child device**

```sh
tedge mqtt pub tedge/child01/inventory/delete ''
```
