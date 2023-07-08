{
    local _self = self,

    proposal3:: {
        local _proposal3 = self,

        getTarget(topic='', meta={}, entities={})::
            # Get cloud target name from the topic name
            # It allows a user to define a custom id, or to default to
            # the topic structure (using the certificate common name as a prefix)
            #
            # Examples:
            #   te/device/main// => <CN>
            #   te/device/main/service/tedge-agent => <CN>:device:main:service:tedge-agent
            #   te/my/full/custom/name => <CN>:my:full:custom:name
            local topic_target = 
                local tmp = std.join("/", [
                    part
                    for part in [meta.device_id] + std.split(topic, '/')[1:5]
                    if !std.isEmpty(part)
                ]);
                # Replace te/device/main with the Common Name value
                if tmp == "%s/device/main" % meta.device_id then
                    meta.device_id
                else
                    tmp
            ;

            local defaultValues = {
                "contents": _proposal3.getEntityType(topic, meta=meta, entities=entities),
            };

            defaultValues + std.get(entities, topic_target, {"@id": std.strReplace(topic_target, "/", ":")})
        ,

        getEntityType(topic='', meta={}, entities={})::
            # Infer the entity type based on the topic
            # Types:
            # * device
            # * child-device
            # * service
            #
            local parts = std.split(topic, "/")[0:5];
            local entity_namespace = parts[1];
            local entity_name = parts[2];
            local component_namespace = parts[3];
            local component_name = parts[4];

            local entity_types = {
                "main": "device",
            };
            local entity_type = std.get(entity_types, entity_name, "child-device");

            local component_types = {
                service: "service",
            };
            local component_type = std.get(component_types, component_namespace, "");
            
            {
                entity: entity_type,
                component: component_type,
                "@id":
                    if std.isEmpty(component_type) then
                        std.join(":", [meta.device_id, entity_namespace, entity_name])
                    else
                        std.join(":", [meta.device_id, entity_type, component_type, ])
                ,
            }
        ,

        getExternalDeviceSource(topic, meta={}, entities={})::
            local target = _proposal3.getTarget(topic, meta=meta, entities=entities);
            {
                externalSource: {
                    externalId: target["@id"],
                    type: "c8y_Serial",
                },
            }
        ,
    },

    # Get the topic prefix, e.g. tedge or tedge/child01
    # depending if the device has a parent or not
    topicPrefix(serial, parent='', prefix='tedge')::
        if std.isEmpty(parent) then
            prefix
        else
            '%s/%s' % [prefix, serial]
    ,

    #
    # Get the external id from a topic.
    # e.g. it will convert:
    #   tedge => device_id
    #   tedge/child01 => device_id/child01
    #
    getSerial(topic, parent='', prefix='tedge')::
        if topic == prefix then
            parent
        else
            local name = 
                if std.startsWith(topic, prefix) then
                    std.lstripChars(topic[std.length(prefix):], "/")
                else
                    topic
            ;
            if parent == name then
                parent
            else
                '%s_%s' % [parent, name]
    ,

    #
    # Get the smart rest topic related to the device
    # e.g. it will convert
    #   tedge => c8y/s/us
    #   tedge/child01 => c8y/s/us/tedge01_child01
    #
    getSmartRestTopic(topic, parent='', smartrest='c8y/s/us', prefix='tedge')::
        local serial = $.getSerial(topic, parent, prefix);
        local tmp =
            if serial == parent then
                ""
            else
                serial
        ;

        std.rstripChars(
            "%s/%s" % [smartrest, tmp],
            "/"
        )
    ,

    getType(topic="/")::
        std.splitLimitR(topic, "/", 1)[1]
    ,

    getExternalId(items=[], sep='_')::
        std.join(sep, items)
    ,

    __padArray(arr, n, default=''):: [
        local len = std.length(arr);
        if i < len then
            arr[i]
        else
            default
        for i in std.range(0,n)
    ],

    measurements:: {
        local _f = self,
        to_value(x):: 
            local parts = _self.__padArray(std.splitLimit(x, ',', 2), 3);
            local key_parts = 
                local p = std.split(parts[0], '.');
                if std.length(p) == 1 then
                    [p[0], p[0]]
                else
                    [p[0], p[1]]
            ;
            {
                [key_parts[0]]+: {
                    [key_parts[1]]+: {
                    value: parts[1],
                    unit: parts[2],
                    }
                },
            }
        ,
        
        from_text(m, sep='\n', init={})::
            std.foldl(
                function(a, b) a + b,
                std.map(_f.to_value, std.split(m, sep)),
                init,
            )
        ,


        is_digit(c)::
            (c > 47 && c < 58) # 0-9
            || c == 46 # .
            || c == 45 # -
            || c == 43 # +
            || c == 69 # E (for exponential values)
        ,
        
        strip_non_numeric(c)::
            if _f.is_digit(c) then
                std.char(c)
            else
                ''
        ,
        
        strip_numeric(c)::
            if _f.is_digit(c) then
                ''
            else
                std.char(c)
        ,
        
        from_str_value(s)::
            {
                value: std.parseJson(
                    std.join('', std.map(_f.strip_non_numeric, std.map(std.codepoint, std.stringChars(s))))
                ),
                unit: std.stripChars(std.join('', std.map(_f.strip_numeric, std.map(std.codepoint, std.stringChars(s)))), ' ')
            }
        ,

        to_meas_value(o, key='', units={})::
            if std.isObject(o) then
                assert 'value' in o : 'If an measurement provides an object value, then it must contain a .value property!';
                {
                    value: std.get(o, 'value'),
                    unit: std.get(o, 'unit', std.get(units, key, '')),
                }
            else
                {value: o, unit: std.get(units, key, '')}
        ,

        from_simple_obj(group, obj, units={}, keyFunc=function(k) k)::
            local _numeric = _f.filter_numeric(obj, units, keyFunc=keyFunc);
            {
                [group]: {
                    [item.key]: _f.to_meas_value(item.value),
                    for item in std.objectKeysValues(_numeric)
                }
            }
        ,

        # build a nested object from a dot notation key to a tested json structure
        # Example:
        #  from_dot(['foo', 'bar'], 1) => {foo:{bar: 1}}
        _to_nested(pathArr, value)::
            std.foldr(function(a, b) {} + {[a]+: b}, pathArr, value)
        ,

        unflatten(obj, init={}, sep='.', limit=1, keyFunc=function(x) std.strReplace(x, '.', '::'))::
            std.foldl(
                function(out, item)
                    local _tmp = std.splitLimit(item.key, sep, limit);
                    local min_depth = limit + 1;
                    local pathArr =
                        if std.length(_tmp) < min_depth then
                            # pad array by repeating the last element the required amout of times
                            _tmp + std.repeat([_tmp[std.length(_tmp)-1]], min_depth - std.length(_tmp))
                        else
                            _tmp
                    ;
                    out + _f._to_nested(
                        std.map(keyFunc, pathArr),
                        if std.isObject(item.value) then
                            item.value
                        else
                            {
                                value: item.value,
                                unit: '',
                            }
                    )
                ,
                std.objectKeysValues(obj),
                init,
            )
        ,

        # Return a new object with only the properties with numeric values (root level only)
        filter_numeric(obj, units={}, keyFunc=function(k) k)::
            if std.isObject(obj) then
                {
                    [item.key]: {value: item.value, unit: std.get(units, keyFunc(item.key), '')}
                    for item in std.objectKeysValues(obj)
                    if std.isNumber(item.value)
                }
            else
                {}
        ,

        # Return an new object with only properties with non-numeric values (root level only)
        filter_meta(obj)::
            if std.isObject(obj) then
                {
                    [item.key]: item.value
                    for item in std.objectKeysValues(obj)
                    if !std.isNumber(item.value)
                }
            else
                {}
        ,

        # Default measurement fields
        defaults(serial, type='thinedge')::
            {
                type: type,
                time: std.native('Now')(),
                externalSource: {
                    externalId: serial,
                    type: 'c8y_Serial',
                },
            }
        ,
    },
}