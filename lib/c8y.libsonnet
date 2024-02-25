{
    service:: {
        # Get status from message.
        # Either object or just plain 1/0
        status(obj, k='status', default='unknown')::
            if std.isObject(obj) then
          std.get(obj, k, default)
        else
          # Map 1 and 0 to up and down
          std.get({'1':'up','0':'down'}, std.toString(obj), default)
    },
    
    operation:: {    
        status(value, defaultValue='FAILED')::
            std.get(
            {
                successful: "SUCCESSFUL",
                failed: "FAILED",
                executing: "EXECUTING",
                pending: "PENDING",
            },
            std.asciiLower(value),
            defaultValue
            ),
        
        type(m, prefix='', defaultType='unknown')::
            local ignoreKeys = ["delivery", "externalSource"];
            local _matches = [
                item.key
                for item in std.objectKeysValues(m)
                if (std.isObject(item.value) || std.isArray(item.value)) && std.startsWith(item.key, prefix) && std.count(ignoreKeys, item.key) == 0
            ];
            if std.length(_matches) > 0 then _matches[0] else defaultType
        ,
    },
}