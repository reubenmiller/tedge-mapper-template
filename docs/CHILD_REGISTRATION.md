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

Or register with a minimum required interval.

```sh
tedge mqtt pub tedge/child10/register/child/child10_03 '{"supportedOperations":["c8y_Restart", "c8y_SoftwareUpdate"],"requiredInterval":10}'
```
