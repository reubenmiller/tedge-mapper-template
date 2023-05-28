# Measurements

## Proposal

Overall proposed measurement topics.

```sh
tedge/measurements
tedge/measurements/{type}
tedge/measurements/{type}/{group}
tedge/measurements/{type}/{group}/{name}
```

## Topic driven measurements


### By Type

Use a simple key/value approach, where nested groups are represented by using dot notation for the keys.

**Topic**

```sh
tedge/measurements
tedge/measurements/{type}
```

**Publish**

```sh
tedge/keyvalue/mytype
```

```json
{
    "temperature": 10.0,
    "environment.humidity": 90.0,
    "environment.pressure": 1032
}
```

**Output**

```json
{
  "type": "mytype",
  "environment": {
    "humidity": {
      "unit": "",
      "value": 90.0
    },
    "pressure": {
      "unit": "",
      "value": 1032
    }
  },
  "temperature": {
    "temperature": {
      "unit": "",
      "value": 20
    }
  }
}
```

**Advantages**

* Easy for consumers to check for presence of a value
* Fairly easy to transform the data into nested json

**Disadvantages**

* Larger message payload due to duplication of group information (e.g. `environment.` needs to be repeated per item in the group)


### Single group

**Topic**

```sh
tedge/measurements/{type}/{group}
```

**Example**

```sh
tedge/measurements/mytype/environment
```

```json
{"temperature": 10.0, "humidity": 20.0}
```

**Output**

```json
{
    "type": "mytype",
    "environment": {
        "temperature": 10.0,
        "humidity": 90,
    }
}
```

## Open Questions


### Bulk measurement creation via text based fields

Support creating a simple key/value text based format.

The payload would follow a simple structure which is easy and quick to parse.

```
time={timestamp}
{group1.name1}={value} {unit}
{group1.name2}={value} {unit}
{group2.other1}={value} {unit}
```

Alternatively, the topic structure could be changed to mimic the collect style, where timestamps can be added to each line

```sh
{group1.name1},{timestamp},{value},{unit}
{group1.name2},{timestamp},{value},{unit}
{group2.other1},{timestamp},{value},{unit}
```

**Topic**

```sh
tedge/measurements-bulk-text/{type}
```

**Example**

```sh
tedge/measurements-bulk-text/mytype
```

```sh
time=12345
environment.temperature=10.0 ˚C
environment.humidity=90 %
```

**Output**

```json
{
  "environment": {
    "humidity": {
      "units": "%",
      "value": "90"
    },
    "temperature": {
      "units": "˚C",
      "value": "10.0"
    }
  },
  "externalSource": {
    "externalId": "sim_tedge01",
    "type": "c8y_Serial"
  },
  "time": "12345",
  "type": "mytype"
}
```

## Summary

### Use flat json objects instead of nested

**Advantages**

* The mapper can transform the flattened json into another structure if they choose to, or leave it as is.

    Supporting all the different format is too complex, and it makes writing a custom mapper more difficult as each format needs to be supported.

    ```json
    {
        // group/name are the same
        "temperature": 10.0,

        // group and name are different
        "environment": {
            "humidity": 10.0
        },
    }
    ```

**Disadvantages**

* Larger payload size as it requires the group name to be repeated on per item


## Dismissed ideas

### Topic driven measurements: Single Value with units

* This is also unnecessarily complex, as it requires components to subscribe to both topics in order to get all values for it.

    ```sh
    tedge/measurements/{type}/{group}/{name}
    tedge/measurements/{type}/{group}/{name}/+
    ```

**Topic**

```sh
tedge/measurements/{type}/{group}/{name}/{unit}
```

**Payload**

```json
10.0
```

**Output**

```json
{
    "type": "{type}",
    "{group}": {
        "{name}": {
            "value": 10.0, "unit": "{unit}"
        },
    }
}
```

**Publish**

```sh
tedge/measurements/v1/temperature/humidity/%
```

```json
10.0
```

**Output**

```json
{
    "type": "v1",
    "temperature": {
        "humidity": {
            "value": 10.0, "unit": "%"
        },
    }
}
```

* Allows users to subscribe to specific values in specific units e.g. percentage vs absolute values
