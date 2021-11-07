const http = require("http");
const url = require("url");
const fetch = require("node-fetch");

const IGNORE_LABELS = new Set(["__name__", "job", "instance"]);

exports.default = async function(config) {
    const seriesBaseUrl = new URL("/api/v1/series", config.prometheusBaseUrl);
    const baseSeriesRes = await fetch(seriesBaseUrl.toString() + "?match[]=" + encodeURIComponent(config.baseLabelQuery));
    const baseSeries = await baseSeriesRes.json();

    const targetSeriesRes = await fetch(seriesBaseUrl.toString() + "?match[]=" + encodeURIComponent(config.targetLabelQuery));
    const targetSeries = await targetSeriesRes.json();

    /** @type {Map<string, string[]>} */
    const baseMetrics = new Map();

    baseSeries.data.forEach(series => {
        const name = series.__name__;
        if (baseMetrics.has(name)) {
            return;
        }
        const labels = Object.keys(series).filter(x => !IGNORE_LABELS.has(x));
        baseMetrics.set(name, labels);
    });

    let covered = 0;
    let accurate = 0;
    /** @type {Set<string>} */
    const found = new Set();
    /** @type {Set<string>} */
    const missing = new Set();
    /** @type {Set<string>} */
    const extra = new Set();
    /** @type {Map<string, { missing: string[], extra: string[] }>} */
    const issues = new Map();

    targetSeries.data.forEach(series => {
       const name = series.__name__;
       if (found.has(name)) {
           return;
       }
       found.add(name);
       if (baseMetrics.has(name)) {
           covered++;
           const labels = Object.keys(series).filter(x => !IGNORE_LABELS.has(x));
           const baseLabels = baseMetrics.get(name);

           const missing = baseLabels.filter(x => !labels.includes(x))
           const extra = labels.filter(x => !baseLabels.includes(x));

           if (missing.length === 0 && extra.length === 0) {
               accurate++;
           } else {
               issues.set(name, { missing, extra });
           }
       } else {
           extra.add(name);
       }
    });

    for (const [key, _] of baseMetrics) {
        if (!found.has(key)) {
            missing.add(key);
        }
    }

    return {
        coverage: covered / baseMetrics.size,
        accuracy: accurate / covered,
        covered,
        accurate,
        totalBase: baseMetrics.size,
        issues: Object.fromEntries(issues),
        missing: Array.from(missing),
        extra: Array.from(extra)
    };
}

exports.formatMarkdown = function(results) {
    // console.log(results)
    const { coverage, accuracy, covered, accurate, totalBase, issues, missing, extra } = results;
    return `**Comparison Results**:

Coverage: ${covered}/${totalBase} (${(coverage * 100).toFixed(2)}%)

Accuracy: ${accurate}/${covered} (${(accuracy * 100).toFixed(2)}%)

${Object.keys(issues).length > 0 ? `<details>
<summary>Issues:</summary>
${Object.keys(issues).map(label => `\`${label}\`: missing ${issues[label].missing.join(", ")}, extra: ${issues[label].extra.join(", ")}`).join("\n")}
</details>` : ""}
        
${missing.length > 0 ? `<details>
<summary>Missing Series:</summary>
${missing.map(label => `* \`${label}\``).join("\n")}
</details>` : ""}

${extra.length > 0 ? `<details>
<summary>Extra Series:</summary>
${extra.map(label => `* \`${label}\``).join("\n")}
</details>` : ""}
`
}
