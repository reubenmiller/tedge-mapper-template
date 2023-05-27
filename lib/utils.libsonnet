{
    # Ref: https://groups.google.com/g/jsonnet/c/1nEJOYmS78I
    get(o, f, default)::
        local get_(o, ks) =
            if ! std.objectHas(o, ks[0]) then
                default
            else if std.length(ks) == 1 then
                o[ks[0]]
            else
                get_(o[ks[0]], ks[1:]);

        get_(o, std.split(f, '.')),
        
    has(o, f)::
        local has_(o, ks) =
            if ! std.objectHas(o, ks[0]) then
                false
            else if std.length(ks) == 1 then
                true
            else
                has_(o[ks[0]], ks[1:]);
        has_(o, std.split(f, '.')),
    
    trimPrefix(s, prefix)::
        if s != '' && prefix != '' then
        if std.startsWith(s, prefix) then
            s[std.length(prefix):]
        else
            s
        else
        s,
    
    recurseReplace(any, from, to)::
        local recurseReplace_(any, from, to) = (
            {
            object: function(x) { [k]: recurseReplace_(x[k], from, to) for k in std.objectFields(x) },
            array: function(x) [recurseReplace_(e, from, to) for e in x],
            string: function(x) std.native('ReplacePattern')(x, from, to),
            #string: function(x) std.strReplace(x, from, to),
            number: function(x) x,
            boolean: function(x) x,
            'function': function(x) x,
            'null': function(x) x,
            }[std.type(any)](any)
        );
        recurseReplace_(any, from, to),
}
