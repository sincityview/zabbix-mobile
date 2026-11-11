package com.zabbix.android.monitor.data

/**
 * Порядок проблем в списке.
 *
 * [Default] — как отдаёт сервер (свежие сверху). [BySeverity] — по убыванию
 * важности. Остальные значения поднимают выбранный уровень в начало, а
 * остальные проблемы идут следом тоже по убыванию важности.
 */
enum class ProblemSort(val firstLevel: Int?) {
    Default(null),
    BySeverity(null),
    Disaster(5),
    High(4),
    Average(3),
    Warning(2),
    Information(1),
    NotClassified(0),
    ;

    companion object {
        fun fromName(name: String?): ProblemSort =
            entries.firstOrNull { it.name == name } ?: Default
    }
}

/**
 * Сортирует проблемы для показа.
 *
 * Сортировки Kotlin стабильные, поэтому внутри одного уровня важности
 * сохраняется серверный порядок — свежие проблемы остаются сверху.
 */
fun sortProblems(problems: List<Problem>, sort: ProblemSort): List<Problem> = when (sort) {
    ProblemSort.Default -> problems

    ProblemSort.BySeverity -> problems.sortedByDescending { it.severityLevel }

    else -> {
        val first = sort.firstLevel
        problems.sortedWith(
            compareByDescending<Problem> { if (it.severityLevel == first) 1 else 0 }
                .thenByDescending { it.severityLevel },
        )
    }
}
