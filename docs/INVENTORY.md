## Inventory updates

### Inventory updates

Updating multiple properties on the root level or non-object updates (like strings, numbers or boolean).

```sh
tedge mqtt pub "tedge/inventory/update" '{"type":"mytype", "custom":{"os":"Debian 11"}}'
tedge mqtt pub "tedge/{child}/inventory/update" '{"type":"mytype", "custom":{"os":"Debian 11"}}'
```

**Main Device**

```sh
tedge mqtt pub tedge/inventory/update '{"custom":{"os":"Debian 11"}}'
```

**Child device**

```sh
tedge mqtt pub tedge/child01/inventory/update '{"custom":{"os":"Debian 11"}}'
```

### Partial inventory updates

Properties on the root level can also be updated by using the topic structure (currently only the root level is supported). This is the preferred method to update fragments as it allows other components to listen to a subset of changes, rather than every inventory update.

```sh
tedge mqtt pub "tedge/inventory/update/{fragment}" '{"os":"Debian 11"}'
tedge mqtt pub "tedge/{child}/inventory/update/{fragment}" '{"os":"Debian 11"}'
```

The fragment in the topic will be used to place the payload like so:

```json
{
    "{fragment}": {
        "os": "Debian 11"
    }
}
```

**Main Device**

```sh
tedge mqtt pub tedge/inventory/update/custom '{"os":"Debian 12"}'
```

**Child device**

```sh
tedge mqtt pub tedge/child01/inventory/update/custom '{"os":"Debian 12"}'
```

### Deleting a fragment

Single fragments can be deleted using the following topics.

```sh
tedge mqtt pub "tedge/inventory/delete/{fragment}" ''
tedge mqtt pub "tedge/{child}/inventory/delete/{fragment}" ''
```

**Main Device**

```sh
tedge mqtt pub tedge/inventory/delete/custom ''
```

**Child device**

```sh
tedge mqtt pub tedge/child01/inventory/delete ''
```
