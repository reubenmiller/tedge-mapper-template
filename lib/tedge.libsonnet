{
    # Get the topic prefix, e.g. tedge or tedge/child01
    # depending if the device has a parent or not
    topicPrefix(serial, parent='', prefix='tedge')::
        if std.isEmpty(parent) then
            prefix
        else
            '%s/%s' % [prefix, serial]
    ,
}