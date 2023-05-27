{
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

    getExternalId(items=[], sep='_')::
        std.join(sep, items)
    ,
}