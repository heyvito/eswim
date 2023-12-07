local debug_level = {
    DISABLED = 0,
    LEVEL_1  = 1,
    LEVEL_2  = 2
}

local DEBUG = debug_level.LEVEL_1

local default_settings =
{
    debug_level  = DEBUG,
    port         = 65333,
    heur_enabled = true,
    heur_regmode = 1,
}

local dprint = function() end
local dprint2 = function() end
local function reset_debug_level()
    if default_settings.debug_level > debug_level.DISABLED then
        dprint = function(...)
            print(table.concat({"Lua:", ...}," "))
        end

        if default_settings.debug_level > debug_level.LEVEL_1 then
            dprint2 = dprint
        end
    end
end
-- call it now
reset_debug_level()

dprint2("Wireshark version = ", get_version())
dprint2("Lua version = ", _VERSION)

----------------------------------------

local eswim = Proto("eswim", "eSWIM Protocol")

-- OPCodes
local opcodes = {
    [0x01] = "BTRP (BootstrapRequest)",
    [0x02] = "BTRR (BootstrapResponse)",
    [0x03] = "BTRA (BootstrapAck)",
    [0x04] = "PING (Ping)",
    [0x05] = "PONG (Pong)",
    [0x06] = "IING (IndirectPing)",
    [0x07] = "IONG (IndirectPong)",
    [0x08] = "SYNC (FullSync)"
}

-- Header Fields
local pf_magic              = ProtoField.uint16("eswim.magic", "Magic Header", base.HEX)
local pf_version_opcode     = ProtoField.uint8("eswim.version_op", "Version & OPCode", base.HEX)
local pf_incarnation        = ProtoField.uint16("eswim.incarnation", "Incarnation", base.DEC)

-- Header -> Version + OPCode fields
local pf_version_opcode_version = ProtoField.new("eswim.version_op.version", "Version", base.DEC)
local pf_version_opcode_opcode = ProtoField.new("eswim.version_op.op", "OPCode", base.HEX)

eswim.fields = {
    pf_magic, pf_version_opcode, pf_version_opcode_version, pf_version_opcode_opcode, pf_incarnation -- Header
}


function eswim.dissector(buffer, pinfo, tree)
    length = buffer:len()
    if length == 0 then return end

    pinfo.cols.protocol = "eSWIM"

    local subtree = tree:add(eswim, buffer(), "eSWIM Protocol")

    -- Header
    -- This function has a complicated form: 'treeitem:add([protofield,] [tvbrange,] value], label)', 
    -- such that if the first argument is a ProtoField or a Proto, the second argument is a TvbRange,
    -- and a third argument is given, it’s a value; but if the second argument is a non-TvbRange,
    -- then it’s the value (as opposed to filling that argument with 'nil', which is invalid for this
    -- function). If the first argument is a non-ProtoField and a non-Proto then this argument
    -- can be either a TvbRange or a label, and the value is not in use.
    
    subtree:add_be(pf_magic, buffer(0, 2))
end