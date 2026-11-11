package com.zabbix.android.monitor.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.put

/**
 * Клиент Zabbix API.
 *
 * Набор данных повторяет страницу «Problems» веб-интерфейса
 * (CScreenProblem::getData в исходниках Zabbix):
 *
 *  - `suppressed = false` — без подавленных (maintenance и ручное подавление);
 *  - `symptom = false`    — без проблем-симптомов (правила корреляции);
 *  - `recent = true`      — вместе с недавно восстановленными, которые в UI
 *                           помечены статусом RESOLVED;
 *  - `monitored = true` и `skipDependent = true` при запросе триггеров —
 *    так отсекаются проблемы выключенных хостов и проблемы зависимых
 *    триггеров, у которых мастер уже в состоянии проблемы.
 *
 * Последние два пункта — причина «дубликатов» в старой версии: на боевом
 * сервере из 162 проблем страница Problems показывает 150, остальные 12 —
 * проблемы выключенного хоста (2) и зависимых триггеров (10).
 */
class ZabbixApi(
    url: String,
    private val token: String,
    private val allowInsecureTls: Boolean = false,
    private val transport: HttpTransport = UrlConnectionTransport,
) {
    private val endpoint: String = ZabbixUrl.normalize(url)

    /**
     * Забирает проблемы с именами хостов.
     *
     * @param limit сколько строк показывать (настройка «лимит отображения»);
     * @param maxFetch предел одного запроса — столько проблем максимум
     *        просматривается за опрос, чтобы не тянуть бесконечный список.
     */
    fun problems(limit: Int = DEFAULT_LIMIT, maxFetch: Int = MAX_FETCH): ProblemsPage {
        val fetch = maxOf(limit, maxFetch)
        val raw = fetchProblems(fetch)
        val visible = if (raw.isEmpty()) emptyList() else resolveHosts(dedupeByTrigger(raw))
        return ProblemsPage(
            shown = visible.take(limit),
            total = visible.size,
            totalKnown = raw.size < fetch,
        )
    }

    /**
     * problem.get. При `limit = 1` — дешёвая проверка подключения и токена.
     */
    private fun fetchProblems(limit: Int): List<Problem> {
        val result = call(
            method = "problem.get",
            params = buildJsonObject {
                put(
                    "output",
                    strings(
                        "eventid", "name", "clock", "severity", "objectid",
                        "acknowledged", "r_eventid", "r_clock",
                    ),
                )
                put("sortfield", strings("eventid"))
                put("sortorder", "DESC")
                put("suppressed", false)
                put("symptom", false)
                put("recent", true)
                put("limit", limit)
            },
        )
        return json.decodeFromJsonElement(result)
    }

    /**
     * Имена хостов из trigger.get.
     *
     * Триггеры, которые сервер не вернул (выключены, удалены или зависимы от
     * триггера в состоянии проблемы), отбрасываются вместе со своими
     * проблемами — ровно как это делает страница Problems.
     */
    private fun resolveHosts(problems: List<Problem>): List<Problem> {
        val ids = problems.map { it.objectid }.filter { it.isNotEmpty() }.distinct()
        if (ids.isEmpty()) return problems

        val hosts = mutableMapOf<String, String>()
        for (chunk in ids.chunked(TRIGGER_CHUNK)) {
            val result = call(
                method = "trigger.get",
                params = buildJsonObject {
                    put("triggerids", strings(chunk))
                    put("output", strings("triggerid"))
                    put("selectHosts", strings("name"))
                    put("monitored", true)
                    put("skipDependent", true)
                },
            )
            val triggers: List<Trigger> = json.decodeFromJsonElement(result)
            triggers.forEach { trigger ->
                trigger.hosts.firstOrNull()?.name
                    ?.takeIf { it.isNotEmpty() }
                    ?.let { hosts[trigger.triggerid] = it }
            }
        }

        return problems.mapNotNull { problem ->
            val host = hosts[problem.objectid] ?: return@mapNotNull null
            problem.copy(hostName = host)
        }
    }

    private fun call(method: String, params: JsonObject): JsonElement {
        val body = json.encodeToString(
            RpcRequest.serializer(),
            RpcRequest(method = method, params = params),
        )
        val raw = transport.post(endpoint, token, body, allowInsecureTls)
        val response = try {
            json.decodeFromString(RpcResponse.serializer(), raw)
        } catch (e: Exception) {
            throw ZabbixException("Некорректный ответ сервера: ${e.message}")
        }
        response.error?.let { throw ZabbixException(it.text) }
        val result = response.result
        return if (result == null || result is JsonNull) JsonArray(emptyList()) else result
    }

    companion object {
        /** Сколько проблем показываем по умолчанию (меняется в настройках). */
        const val DEFAULT_LIMIT = 100

        /** Верхняя граница настройки лимита. */
        const val MAX_LIMIT = 10_000

        /**
         * Сколько проблем максимум просматриваем за один опрос: `total` в
         * счётчике должен быть честным, поэтому список забирается целиком,
         * а лимит отображения режет уже готовый список.
         */
        const val MAX_FETCH = 2_000

        /** Zabbix отдаёт не больше 500 id за один trigger.get на практике. */
        private const val TRIGGER_CHUNK = 500

        /** Массив строк для params: у JsonArrayBuilder нет перегрузки add(String). */
        private fun strings(values: List<String>): JsonArray =
            JsonArray(values.map { JsonPrimitive(it) })

        private fun strings(vararg values: String): JsonArray = strings(values.toList())

        private val json = Json {
            ignoreUnknownKeys = true
            explicitNulls = false
            // Без этого kotlinx.serialization выбрасывает поля, совпадающие со
            // значением по умолчанию: запрос уходил без "jsonrpc": "2.0", и
            // сервер отвечал «the parameter "jsonrpc" is missing».
            encodeDefaults = true
        }

        /**
         * Оставляет по одной проблеме на триггер.
         *
         * В таблице problem Zabbix может держать несколько строк одного
         * триггера (восстановленную — пока её не убрал housekeeper). Запрос
         * отсортирован по eventid DESC, поэтому первая запись — самая свежая.
         */
        fun dedupeByTrigger(problems: List<Problem>): List<Problem> {
            if (problems.size < 2) return problems
            val seen = HashSet<String>(problems.size)
            return problems.filter { problem ->
                val key = problem.objectid.ifEmpty { "eventid:${problem.eventid}" }
                seen.add(key)
            }
        }
    }
}
