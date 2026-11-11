package com.zabbix.android.monitor.data

import java.net.URI

/**
 * Нормализация адреса сервера Zabbix.
 *
 * Порт `NormalizeURL` из Go-версии (data/conn.go), поведение совпадает
 * посимвольно, потому что от этого зависит и клиент API, и совместимость
 * сохранённых настроек:
 *
 *  - без схемы подставляется https://;
 *  - путь дополняется /api_jsonrpc.php, если он не указывает на .php;
 *  - query и fragment отбрасываются (их в адресе API быть не должно).
 *
 * Класс без зависимостей от Android — проверяется обычными JVM-тестами.
 */
object ZabbixUrl {

    const val API_PATH = "/api_jsonrpc.php"

    fun normalize(raw: String): String {
        val trimmed = raw.trim()
        if (trimmed.isEmpty()) return ""

        val withScheme = if (trimmed.contains("://")) trimmed else "https://$trimmed"

        val uri = try {
            URI(withScheme)
        } catch (_: Exception) {
            return withScheme
        }

        val host = uri.host
        if (host.isNullOrEmpty()) return withScheme

        val path = uri.path.orEmpty().trimEnd('/')
        val apiPath = when {
            path.isEmpty() -> API_PATH
            path.endsWith(".php") -> path
            else -> path + API_PATH
        }

        return try {
            URI(uri.scheme, uri.userInfo, host, uri.port, apiPath, null, null).toString()
        } catch (_: Exception) {
            withScheme
        }
    }

    /** Адрес использует http:// — токен пойдёт по сети открытым текстом. */
    fun isCleartext(raw: String): Boolean = normalize(raw).lowercase().startsWith("http://")
}
