# tedge-mapper-template

Just some free experiments (PoCs) looking at how a template based mapper could look like.

The idea is to have a mapper which supports loading the "routes" from configuration (currently file-based), and applying the routes to different messages.

A route defines which MQTT topic it should be listening to and the transformation template it should apply to any received messages. A user can define multiple routes, however currently only 1 route is allowed per topic. However you can get around this limitation by using one-route which is listening to the common topic and it transform the message to publish to a new unique topics where the other routes can configure to listen to.

The following parameters are configurable within routes:

* Source topic that the route should be listening to
* Topic of the outgoing message
* Control whether the outgoing message should be published or not (by setting the `.skip` property of the message)
* Template to be applied to generate the outgoing message. The template has access to the incoming message as well as some additional configuration which can be used in the message transformation.

In addition to the route configuration, there are also some prevention mechanism to prevent errors when creating custom routes.

* Recursive message counter (to prevent infinite message loops)
* Limit publishing rate (to prevent spamming)
* Control if a message is allowed to be processed by other routes or not (via the `.end` property). Idea is to also allow the route to decide if it's messages are allowed to be accepted by other routes or not.


### Use-cases

The following use-cases are possible using the configurable mapper.

* Pre process messages before other components use it (e.g. fix something automatically)
* Split message processing logic (e.g. `A -> B -> C`)
* React to other messages, e.g. create events on operation status updates
* Provide state in context which is reusable across other messages

## Design

Below shows a rough diagram of the flow of a single route which is listening to a specific topic.

```mermaid
flowchart TD

    Init --> LoadRoutes
    LoadRoutes --> SubscribeToTopics
    SubscribeToTopics --> Broker

    Broker -->|OnMessage| Transform
    Transform --> PostProcessor

    PostProcessor --> Publisher{Publish?}
    Publisher -->|Yes| Broker[Broker]
    Publisher -->|No| DoNothing[DoNothing]
```

## Template language

Currently only [jsonnet](https://jsonnet.org/) is supported. However jsonnet is a very flexible template language which allows you to run your own functions.

The example below shows a more complicated scenario where a template is used to replace any references to the internal Cumulocity IoT URL with the public URL (as read from the meta information). It uses a custom function which does a recursive search for any strings which contain a wildcard pattern (though the `_.ReplacePattern` function is provided by the application and not the jsonnet library (just in case if you try to run the template on the jsonnet website ;))

```jsonnet
local recurseReplace(any, from, to) = (
  {
    object: function(x) { [k]: recurseReplace(x[k], from, to) for k in std.objectFields(x) },
    array: function(x) [recurseReplace(e, from, to) for e in x],
    string: function(x) _.ReplacePattern(x, from, to),
    number: function(x) x,
    boolean: function(x) x,
    'function': function(x) x,
    'null': function(x) x,
  }[std.type(any)](any)
);

# THIS PART IS THE OUTGOING MESSAGE!
{
  message: recurseReplace(message, 'https?://\\bt\\d+\\.cumulocity.com', 'https//' + std.get(meta, 'c8y_http', '')),
  end: true,
  topic: topic,
  skip: false,
}
```

To make the template language more useful, additional variables are also injected into the template each time the template is applied to an incoming message.

|Variable|Description|Example|
|----|----|----|
|`topic`|Topic of the incoming message|`c8y/s/ds/524`|
|`message`|Payload of incoming message (most of the time this is JSON but it can be CSV|`{}`|
|`meta`|Additional meta information which can be used within the templates (e.g. access environment variables `meta.env.<ENV_VARIABLE>`)|`{"device_id":"mydevice","env":{"TEDGE_ROUTE_CUSTOM_DATA":"foo/bar"}}`|
|`ctx`|Internal Routing Context, e.g. how many levels of routes has the message or derivatives of the message|`{"lvl":0}`|
|`_`|Object providing some additional functions like `_.Now()` to get the current timestamp in RFC3334 format|

You can see the exact jsonnet templates used (including the injected runtime information) by specifying the `--debug` flag.

For example, starting the application with `--debug` will print out the full jsonnet template to the console.

```sh
go run main.go --debug
```

Below shows an example of full jsonnet template which is applied to the incoming message. 

```jsonnet
local topic = 'c8y/s/ds/524';
local _input = {"id":"524","serial":"DeviceSerial","content":{"url":"http://www.my.url","type":"type"},"payload":"524,DeviceSerial,http://www.my.url,type"};
local message = if std.isObject(_input) then _input + {_ctx:: null} else _input;
local ctx = {lvl:0} + std.get(_input, '_ctx', {});
local meta = {"device_id":"test","env":{"C8Y_BASEURL":"https://example.cumulocity.com"}};

local _ = {Now: function() std.native('Now')(), ReplacePattern: function(s, from, to='') std.native('ReplacePattern')(s, from, to),};

###

{
    message: message.content,
    topic: 'tedge/operations/req/' + message.serial + '/' + 'download_config',
}
 + {message+: {_ctx: ctx + {lvl: std.get(ctx, 'lvl', 0) + 1}}}

```

When the above template is evaluated, the following JSON data is produced. This will be the data which is interpreted by the PostProcessor and published as a MQTT message. It shows that the data structure includes the `.topic` field which is used to tell where the `.message` should be published to. There are additional properties which can also be used to customize the handling of this message.

*Output: Template output*


```json
{
  "message": {
    "_ctx": {
      "lvl": 1
    },
    "type": "type",
    "url": "http://www.my.url"
  },
  "topic": "tedge/operations/req/DeviceSerial/download_config"
}
```

### Route output format

Each route should output a single object which contains information about how the evaluated template should be processed by the runner.

For example the following show a minimal example of such a route output:

```json
{
  "message": {
    "_ctx": {
      "lvl": 1
    },
    "type": "type",
    "url": "http://www.my.url"
  },
  "topic": "tedge/operations/req/DeviceSerial/download_config"
}
```

|Property|Type|Description|
|---|---|---|
|`.message`|object|json object containing the payload of either the MQTT or HTTP Request that will be sent by the runner|
|`.topic`|string|MQTT topic that the `.message` should be sent to. The inclusion of the `.topic` indicates that the message will be sent via MQTT|
|`.skip`|boolean|If true, the output message will be ignored by the runner and the `.message` will not be sent via MQTT or REST|
|`.context`|boolean|Indicates if the context property `_ctx` of the `.message` should be included in the outgoing message or not. The `_ctx` is added automatically by the template engine to add message tracing|
|`.end`|boolean|The outgoing message should not be processed by any other routes. This only works if `.context` is NOT set to `false`|
|`.api`|object|Object containing information about which HTTP Request should be sent. Inclusion of the `.api` property indicates that a HTTP Request will be sent instead of an MQTT message (see below for the expected properties of the object|
|`.api.method`|string|HTTP Request Method, e.g. `GET`, `POST`, `PUT`|
|`.api.path`|string|HTTP Request path, e.g. `devicecontrol/operations/12345`|
|`.raw_message`|string|String based MQTT payload (e.g. good for c8y SmartREST 2.0 messsages). Note: this could be deprecated in the future once the `.message` can handle both strings and object formats|
|`.updates[]`|array of objects|Additional MQTT messages that will also be sent, however these are intended for messages that will not be processed by other routes.|
|`.updates[].topic`|string|MQTT topic for the update message|
|`.updates[].message`|string|MQTT payload for the update message. Can be a string or an object. It will not contain any reference to the context property|
|`.updates[].skip`|boolean|The update message will be ignored if this is set to `true`|


## Caveats

* Template based mapping will likely be too slow for high throughput messages (this is a tradeoff for having high configuration)
* Templates are loaded from a yaml spec which contains jsonnet templates embedded

## Getting started

After checking out the project you can get everything up and running using the following commands.

```sh
go run main.go
```

By default it will listen to the MQTT broker on `localhost:1883`, however it can be changed. Just checkout the options in the help, e.g.

```sh
go run main.go --help
```

`tedge-mapper-template` will also load all of the routes in the `./routes` directory to help you get an idea what are some of the possibilities.


Once the application has subscribed to the MQTT broker, then you can open another console, and try publishing to a topic which will trigger one of the matching routes. Below is publishing a message using `mosquitto_pub` to the `c8y/s/ds` topic.

```sh
mosquitto_pub -t 'c8y/s/ds' -m '524,DeviceSerial,http://www.my.url,type'
```

Check the output of the `tedge-mapper-template`, and you will see that there activity there showing the processing of some messages:

```log
2023-05-18T21:57:33+02:00 INF Starting listener
2023-05-18T21:57:33+02:00 INF Registering route. name=c8y-operation-smartrest topic=c8y/s/ds
2023-05-18T21:57:33+02:00 INF Registering route. name=shell-operation topic=c8y/s/ds/511
2023-05-18T21:57:33+02:00 INF Registering route. name=download-config-operation topic=c8y/s/ds/524
2023-05-18T21:57:33+02:00 INF Registering route. name=firmware-update-operation topic=c8y/s/ds/515
2023-05-18T21:57:33+02:00 INF Registering route. name=software-update-operation topic=c8y/s/ds/529
2023-05-18T21:57:33+02:00 INF Ignoring route marked as skip. name="Cumulocity Operation Mapper Without Preprocessor" topic=c8y/s/ds
2023-05-18T21:57:33+02:00 INF Ignoring route marked as skip. name="simple measurements" topic=tedge/measurements
2023-05-18T21:57:33+02:00 INF Registering route. name="complex measurements" topic=tedge/measurements/+
2023-05-18T21:57:33+02:00 INF Registering route. name="Trigger event from measurement" topic=tedge/measurements
2023-05-18T21:57:33+02:00 INF Registering route. name="Modify urls" topic=tedge/operations/req/config_update
2023-05-18T21:57:35+02:00 INF Route activated on message. route=c8y-operation-smartrest topic=c8y/s/ds message=524,DeviceSerial,http://www.my.url,type
2023-05-18T21:57:35+02:00 INF Publishing new message. topic=c8y/s/ds/524 message=524,DeviceSerial,http://www.my.url,type
2023-05-18T21:57:37+02:00 INF Route activated on message. route=download-config-operation topic=c8y/s/ds/524 message=524,DeviceSerial,http://www.my.url,type
2023-05-18T21:57:37+02:00 INF Publishing new message. topic=tedge/operations/req/DeviceSerial/download_config message="{\"_ctx\":{\"lvl\":1},\"type\":\"type\",\"url\":\"http://www.my.url\"}"
```

The above log output shows that the `c8y-operation-operation-smartrest` route reacted to an incoming SmartREST message. The route then transformed the message and published a new message on a different topic which includes the SmartREST template id as other routes are listening to specific SmartREST template ids.

The `download-config-operation` route, then reacts and transforms the CSV message into JSON. Below shows a pretty printed version of the outgoing message from this router.

```json
{
    "_ctx": {
        "lvl": 1
    },
    "type": "type",
    "url": "http://www.my.url"
}
```

The `_ctx` fragment is automatically added to the message payload to try and prevent infinite loops. Each time the JSON payload goes through a route, the `_ctx.lvl` will increase by one. Currently the route counter is only added to JSON message (not CSV) due to a limitation. In the future only JSON formats will be supported, so this should not be too limiting. The other two properties, `type` and `url` have been added by the route during the conversion from CSV to JSON (using the in-built preprocessor block). Once the message is in the JSON format, it is much easier for plugins to handle the data, and add/remove fragments as needed.

## Checking routes offline

Routes allow users to transform incoming messages and generate new messages as a result. This means you can also chain routes together by configuring one route to publish to another route. Even complicated changes like `A -> B -> C -> D` are possible.

To make it easier to develop complex chained routes, or if you are just wanting to experiment with the template language, then you can check how your routes respond to different topics/message offline using the following command:

```sh
go run main.go routes check -t 'c8y/devicecontrol/notifications' -m '{}' --silent
```

*Output*

```sh
Route: c8y-devicecontrol-notifications (c8y/devicecontrol/notifications)

Input Message
  topic:    c8y/devicecontrol/notifications

Output Message
  topic:    c8y/devicecontrol/notifications/not-set/unknown
  end:      false

{
  "_ctx": {
    "deviceID": "",
    "lvl": 1,
    "opType": "unknown",
    "serial": "not-set"
  },
  "payload": {}
}

Route: unknown-operation (c8y/devicecontrol/notifications/+/unknown)

Input Message
  topic:    c8y/devicecontrol/notifications/not-set/unknown

Output Message
  topic:    tedge/events/unknown_operation
  end:      false

{
  "_ctx": {
    "deviceID": "",
    "lvl": 2,
    "opType": "unknown",
    "serial": "not-set"
  },
  "_request": {},
  "text": "Unknown operation type. Check the _request fragment to inspect the original message"
}
```

Or you can provide more complicated JSON via the `--message/-m` flag.

```sh
go run main.go routes check -t 'c8y/devicecontrol/notifications' -m '{"c8y_Command":{"text":"ls -l"}}' --silent
```

## Building

You can build the binaries for a range of targets by using the following command, though before you run it, you need to install some tooling which is used to run the project's tasks.

* Install [just](https://just.systems/man/en/chapter_5.html)
* Install [goreleaser](https://goreleaser.com/install/)

Once you've installed the above tools, then you can build the project using:

```sh
just build
```

## Add json handling of the notifications

Note: This is only a proof of concept, it does not mean that everything will work if you follow these instructions. At the moment the only operation that really works is the install/remove software.

Edit the `c8y-bridge.conf` to add a new bridge configuration so that the bridge will receive the Cumulocity operations in the json format.

```sh
/etc/tedge/mosquitto-conf/c8y-bridge.conf
```

```sh
topic devicecontrol/notifications/# in 2 c8y/ ""
```

Additionally you can comment out the subscription to the `s/ds` topic. Afterwards it should look like this.

```sh
#topic s/ds in 2 c8y/ ""
```

Finally you will have to restart the mosquitto service.

```
sudo systemctl restart mosquitto
```

To Test it out, try installing some new software
