{
    local _self = self,

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

    lookupID(localName, meta={})::
        local device_mapping = import 'device_mapping.json';
        if localName in device_mapping then
            std.get(device_mapping, localName, localName)
        else
            std.join('_', [meta.device_id, localName])
    ,

    getExternalDeviceId(topic, meta={})::
        local device_mapping = import 'device_mapping.json';
        local localName = std.split(topic, '/')[1];
        _self.lookupID(localName, meta)
    ,
    getExternalDeviceSource(topic, meta={})::
        {
            externalSource: {
                externalId: _self.getExternalDeviceId(topic, meta),
                type: "c8y_Serial",
            },
        }
    ,

    getExternalServiceId(topic, meta={})::
        local parts = std.split(topic, '/');
        '%s_%s' % [
            _self.lookupID(parts[1], meta),
            parts[3]
        ]
    ,
    getExternalServiceSource(topic, meta={})::
        {
            externalSource: {
                externalId: _self.getExternalServiceId(topic, meta),
                type: "c8y_Serial",
            },
        }
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