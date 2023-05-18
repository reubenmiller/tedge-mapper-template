local topic = '';
local message = '';
local meta = {type:''};

###

{
    message: {
        c8y_Command: message,
    },
    topic: topic + "/" + meta.type,
    skip: if std.endsWith(topic, "/measurements") then true else false,
}