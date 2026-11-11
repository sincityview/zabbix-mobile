package com.zabbix.android.monitor.data

import org.junit.Assert.assertEquals
import org.junit.Test

class ProblemSortTest {

    private fun problem(eventid: String, severity: Int) = Problem(
        eventid = eventid,
        objectid = "t$eventid",
        severity = severity.toString(),
        name = "P$eventid",
    )

    // сервер уже отдаёт свежие сверху — этот порядок и есть «по умолчанию»
    private val serverOrder = listOf(
        problem("5", 1),
        problem("4", 5),
        problem("3", 2),
        problem("2", 5),
        problem("1", 0),
    )

    @Test
    fun `по умолчанию порядок сервера не меняется`() {
        assertEquals(
            listOf("5", "4", "3", "2", "1"),
            sortProblems(serverOrder, ProblemSort.Default).map { it.eventid },
        )
    }

    @Test
    fun `по важности — от Disaster к неклассифицированным, свежие остаются первыми`() {
        assertEquals(
            listOf("4", "2", "3", "5", "1"),
            sortProblems(serverOrder, ProblemSort.BySeverity).map { it.eventid },
        )
    }

    @Test
    fun `выбранный уровень поднимается в начало, остальные идут по важности`() {
        // Warning (2) — единственная проблема 3, она встаёт первой
        assertEquals(
            listOf("3"),
            sortProblems(serverOrder, ProblemSort.Warning).take(1).map { it.eventid },
        )
        assertEquals(
            listOf("3", "4", "2", "5", "1"),
            sortProblems(serverOrder, ProblemSort.Warning).map { it.eventid },
        )
    }

    @Test
    fun `пустой список и один элемент не ломают сортировку`() {
        assertEquals(emptyList<Problem>(), sortProblems(emptyList(), ProblemSort.BySeverity))
        assertEquals(1, sortProblems(listOf(problem("1", 3)), ProblemSort.Disaster).size)
    }

    @Test
    fun `неизвестное значение настройки читается как по умолчанию`() {
        assertEquals(ProblemSort.Default, ProblemSort.fromName(null))
        assertEquals(ProblemSort.Default, ProblemSort.fromName("мусор"))
        assertEquals(ProblemSort.Disaster, ProblemSort.fromName("Disaster"))
    }
}
