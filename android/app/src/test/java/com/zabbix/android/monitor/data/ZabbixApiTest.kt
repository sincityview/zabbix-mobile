package com.zabbix.android.monitor.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/** Транспорт-заглушка: запоминает запросы и отдаёт заготовленные ответы. */
private class FakeTransport(private val handler: (String) -> String) : HttpTransport {
    val requests = mutableListOf<Pair<String, JsonObject>>()
    val bodies = mutableListOf<String>()

    override fun post(url: String, token: String, body: String, allowInsecureTls: Boolean): String {
        bodies.add(body)
        val envelope = Json.parseToJsonElement(body).jsonObject
        val method = envelope["method"]!!.jsonPrimitive.content
        val params = envelope["params"]!!.jsonObject
        requests.add(method to params)
        return handler(method)
    }
}

private fun ok(result: String) = """{"jsonrpc":"2.0","result":$result,"id":1}"""

private fun apiError(code: Int, message: String, data: String) =
    """{"jsonrpc":"2.0","error":{"code":$code,"message":"$message","data":"$data"},"id":1}"""

class ZabbixApiTest {

    @Test
    fun `problem_get спрашивает то же, что страница Problems`() {
        val transport = FakeTransport { method ->
            when (method) {
                "problem.get" -> ok(
                    """[{"eventid":"1","objectid":"100","name":"ICMP down","clock":"1700000000",
                       "severity":"3","acknowledged":"0","r_eventid":"0","r_clock":"0"}]""",
                )
                "trigger.get" -> ok("""[{"triggerid":"100","hosts":[{"name":"host-a"}]}]""")
                else -> error("неожиданный метод $method")
            }
        }

        val problems = ZabbixApi("zabbix.local", "token", false, transport).problems(limit = 50)

        assertEquals(1, problems.size)
        assertEquals("host-a", problems[0].hostName)

        val (problemMethod, problemParams) = transport.requests.first()
        assertEquals("problem.get", problemMethod)
        assertFalse("подавленные должны быть скрыты", problemParams["suppressed"]!!.jsonPrimitive.boolean)
        assertFalse("симптомы должны быть скрыты", problemParams["symptom"]!!.jsonPrimitive.boolean)
        assertTrue("недавно восстановленные нужны", problemParams["recent"]!!.jsonPrimitive.boolean)
        assertEquals(50, problemParams["limit"]!!.jsonPrimitive.int)
        assertEquals("DESC", problemParams["sortorder"]!!.jsonPrimitive.content)

        val output = problemParams["output"]!!.jsonArray.map { it.jsonPrimitive.content }
        assertTrue(output.containsAll(listOf("eventid", "objectid", "severity", "r_eventid", "r_clock")))

        val triggerParams = transport.requests.last().second
        assertTrue("только отслеживаемые триггеры", triggerParams["monitored"]!!.jsonPrimitive.boolean)
        assertTrue("зависимые триггеры отсекаются", triggerParams["skipDependent"]!!.jsonPrimitive.boolean)

        // Регрессия: без encodeDefaults kotlinx.serialization выкидывал
        // "jsonrpc": "2.0", и Zabbix отвечал «the parameter jsonrpc is missing»
        assertTrue(
            "в теле запроса должен быть jsonrpc",
            transport.bodies.first().contains("\"jsonrpc\":\"2.0\""),
        )
    }

    @Test
    fun `отбрасывает проблемы триггеров, которых нет среди monitored`() {
        val transport = FakeTransport { method ->
            when (method) {
                "problem.get" -> ok(
                    """[
                        {"eventid":"2","objectid":"200","name":"Disk full","clock":"1700000001","severity":"5"},
                        {"eventid":"1","objectid":"100","name":"ICMP down","clock":"1700000000","severity":"3"}
                    ]""",
                )
                // Триггер 200 сервер не вернул: выключен или зависим от проблемы
                "trigger.get" -> ok("""[{"triggerid":"100","hosts":[{"name":"host-a"}]}]""")
                else -> error("неожиданный метод $method")
            }
        }

        val problems = ZabbixApi("zabbix.local", "token", false, transport).problems()

        assertEquals(1, problems.size)
        assertEquals("1", problems[0].eventid)
        assertEquals("host-a", problems[0].hostName)
    }

    @Test
    fun `оставляет по одной проблеме на триггер`() {
        val transport = FakeTransport { method ->
            when (method) {
                // сервер отдаёт по eventid DESC, поэтому 9 — самая свежая
                "problem.get" -> ok(
                    """[
                        {"eventid":"9","objectid":"100","name":"CPU high","clock":"1700000900","severity":"4"},
                        {"eventid":"1","objectid":"100","name":"CPU high","clock":"1700000000","severity":"4"}
                    ]""",
                )
                "trigger.get" -> ok("""[{"triggerid":"100","hosts":[{"name":"host-a"}]}]""")
                else -> error("неожиданный метод $method")
            }
        }

        val problems = ZabbixApi("zabbix.local", "token", false, transport).problems()

        assertEquals(1, problems.size)
        assertEquals("9", problems[0].eventid)
    }

    @Test
    fun `помечает восстановленные проблемы`() {
        val transport = FakeTransport { method ->
            when (method) {
                "problem.get" -> ok(
                    """[{"eventid":"1","objectid":"100","name":"ICMP down","clock":"1700000000",
                       "severity":"2","acknowledged":"1","r_eventid":"2","r_clock":"1700000300"}]""",
                )
                "trigger.get" -> ok("""[{"triggerid":"100","hosts":[{"name":"host-a"}]}]""")
                else -> error("неожиданный метод $method")
            }
        }

        val problem = ZabbixApi("zabbix.local", "token", false, transport).problems().single()

        assertTrue(problem.resolved)
        assertEquals(2, problem.severityLevel)
        assertEquals("host-a", problem.hostName)
    }

    @Test
    fun `ошибка API приходит как ZabbixException`() {
        val transport = FakeTransport { apiError(-32602, "Invalid params.", "bad token") }

        val error = assertThrows(ZabbixException::class.java) {
            ZabbixApi("zabbix.local", "token", false, transport).problems()
        }

        assertTrue(error.message.orEmpty().contains("bad token"))
        assertTrue(error.message.orEmpty().contains("-32602"))
    }

    @Test
    fun `URL запроса нормализуется до api_jsonrpc_php`() {
        var calledUrl = ""
        val transport = HttpTransport { url, _, _, _ ->
            calledUrl = url
            ok("[]")
        }

        ZabbixApi("zabbix.local/zabbix", "token", false, transport).problems()

        assertEquals("https://zabbix.local/zabbix/api_jsonrpc.php", calledUrl)
    }
}

class ZabbixTimeTest {

    @Test
    fun `форматирует unix-время и терпит мусор`() {
        val formatted = ZabbixTime.format("1700000000")
        assertEquals(14, formatted.length)
        assertTrue(ZabbixTime.format("1700000000", ZabbixTime.TIME_OF_DAY).length == 5)
        assertEquals("not-a-number", ZabbixTime.format("not-a-number"))
    }
}
