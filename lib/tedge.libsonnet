{
    isChild(ctx, meta)::
        local device_id = std.get(meta, 'device_id', '');
        ctx.serial == device_id || ctx.serial == 'not-set'
    ,

    topic_prefix(ctx, meta)::
        local device_id = std.get(meta, 'device_id', '');
        if $.isChild(ctx, meta) then
            "tedge/%s" % std.get(meta, 'device_id', '')
        else
            "tedge"
    ,

    topicPrefix(serial, parent='', prefix='tedge')::
        if std.isEmpty(parent) then
            prefix
        else
            '%s/%s' % [prefix, serial]
    ,

    meta:: {
        device_id: '',
    }
}