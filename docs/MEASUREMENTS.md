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

Topic driven measurement are a great way to represent the measurement structure via the MQTT topics.

Splitting a message into different groups has an advantages because it allows other components to subscribe to a subset of measurement based on the type/group/name instead of listening to every measurement (which is very inefficient and could cause higher resource usage to filter out irrelevant measurements).

For high ingestion use-cases, messages can be publish to the type topic which sends measurement with multiple groups to one topic. These types of large measurements will be harder for other components to subscribe to.

### Units

A simple lookup mechanism has been added to show an example how units could be added to the measurements via a static json file. The units file can be imported in the templates, and the units will be looked up via the `group + '.' + name` syntax.

Below shows an example of the `units.json` used by the routes.

```json
{
  "environment.humidity": "%",
  "environment.temperature": "˚C"
}
```

The `units.json` file could be updated via the tedge configuration plugin, or by just manually replacing the file (though currently the service will need to be reloaded for it to take effect).

### By type

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
      "value": 10.0
    }
  }
}
```

**Advantages**

* Easy for consumers to check the presence of a value
* Fairly easy to transform the data into nested json if nested structure is required

**Disadvantages**

* Larger message payload due to duplication of group information (e.g. `environment` needs to be repeated per item in the group)


### By type/group

**Topic**

```sh
tedge/measurements/{type}/{group}
```

**Example**

```sh
tedge/measurements/mytype/environment
```

```json
{"temperature": 10.0, "humidity": 90}
```

**Output**

```json
{
    "type": "mytype",
    "environment": {
        "temperature": {
            "value": 10.0,
            "unit": ""
        },
        "humidity": {
            "value": 90,
            "unit": ""
        }
    }
}
```

### By type/group/name

Publish a measurement with a single value.

**Topic**

```sh
tedge/measurements/{type}/{group}/{name}
```

**Example**

```sh
tedge/measurements/mytype/environment/temperature
```

Either publish using a text based payload

```sh
10.0
```

Or a json payload

```json
{"value": 10.0}
```

Both payloads will produce the same output.

**Output**

```json
{
    "type": "mytype",
    "environment": {
        "temperature": {
            "value": 10.0,
            "unit": ""
        }
    }
}
```

## Misc. topics

### Bulk measurement creation via text based payloads

Support creating measurement using a csv format.

The payload would follow a simple csv structure which is easy and quick to parse.

Below shows the format of such a text-based message. There is one "special" line `time,{timestamp}` which when present will control the timestamp to use. If the payload does not contain a timestamp, then the current time when the message was received by the mapper will be used.

```csv
time,{timestamp}
{group1.name1},{value},{unit}
{group1.name2},{value},{unit}
{group2.other1},{value},{unit}
```

Alternatively, the format could be changed to use a pure-csv format, however it would mean duplicating the timestamp on each line, as the measurements should already be grouped by a single timestamp.

```csv
{timestamp},{group1.name1},{value},{unit}
{timestamp},{group1.name2},{value},{unit}
{timestamp},{group2.other1},{value},{unit}
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
time,12345
environment.temperature,10.0,˚C
environment.humidity,90,%
```

**Output**

```json
{
  "environment": {
    "humidity": {
      "unit": "%",
      "value": "90"
    },
    "temperature": {
      "unit": "˚C",
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

## Design notes

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
* Easier for components to create the payload as you could build the json object yourself without a json library (as it it essentially key/values with a fixed prefix and postfix)

**Disadvantages**

* Larger payload size as it requires the group name to be repeated on per item


## Dismissed ideas

### Topic driven measurements: Single Value with units

Include the units in the topic structure when publishing to a single measurement.

**Topic**

```sh
tedge/measurements/{type}/{group}/{name}/{unit}
```

**Payload**

```json
10.0
```

**Template**

```json
{
    "type": "{type}",
    "{group}": {
        "{name}": {
            "value": 10.0,
            "unit": "{unit}"
        }
    }
}
```

**Example**

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
            "value": 10.0,
            "unit": "%"
        }
    }
}
```

**Advantages**

* Allows users to subscribe to specific values in specific units e.g. percentage vs absolute values

**Disadvantages**

* This is also unnecessarily complex, as it requires components to subscribe to both topics in order to get all values for it.

    ```sh
    tedge/measurements/{type}/{group}/{name}
    tedge/measurements/{type}/{group}/{name}/+
    ```
