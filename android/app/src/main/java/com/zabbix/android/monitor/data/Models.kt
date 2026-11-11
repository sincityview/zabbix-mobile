package com.zabbix.android.monitor.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

/**
 * Проблема из problem.get.
 *
 * Все поля — строки: Zabbix отдаёт идентификаторы и числа строками, и
 * приводить их к Int на разборе значит падать на неожиданном значении.
 */
@Serializable
data class Problem(
    val eventid: String = "",
    val objectid: String = "",
    val name: String = "",
    val clock: String = "",
    val severity: String = "0",
    val acknowledged: String = "0",
    @SerialName("r_eventid") val recoveryEventId: String = "",
    @SerialName("r_clock") val recoveryClock: String = "",
    /** Заполняется из trigger.get; у отброшенных проблем остаётся пустым. */
    @SerialName("host_name") val hostName: String = "",
) {
    /**
     * Проблема уже восстановлена. Zabbix по умолчанию отдаёт недавно
     * восстановленные вместе с активными, и без этой пометки одна и та же
     * проблема выглядела в списке дважды.
     */
    val resolved: Boolean
        get() = recoveryEventId.isNotEmpty() && recoveryEventId != "0"

    /** 0..5, либо -1 если сервер вернул что-то неожиданное. */
    val severityLevel: Int
        get() = severity.toIntOrNull()?.takeIf { it in 0..5 } ?: -1
}

@Serializable
data class Trigger(
    val triggerid: String = "",
    val hosts: List<TriggerHost> = emptyList(),
)

@Serializable
data class TriggerHost(val name: String = "")

// --- JSON-RPC ---

@Serializable
data class RpcRequest(
    val jsonrpc: String = "2.0",
    val method: String,
    val params: JsonObject,
    val id: Int = 1,
)

@Serializable
data class RpcResponse(
    val result: JsonElement? = null,
    val error: RpcError? = null,
)

@Serializable
data class RpcError(
    val code: Int = 0,
    val message: String = "",
    val data: String = "",
) {
    val text: String
        get() = when {
            data.isNotEmpty() -> "Zabbix API [$code]: $message ($data)"
            message.isNotEmpty() -> "Zabbix API [$code]: $message"
            else -> "Zabbix API [$code]"
        }
}

/** Ошибка, пришедшая в теле JSON-RPC ответа. */
class ZabbixException(message: String) : Exception(message)
