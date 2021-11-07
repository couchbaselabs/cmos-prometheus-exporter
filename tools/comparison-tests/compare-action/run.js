const compare = require("./compare");

(async function(){
    const results = await compare.default({
        prometheusBaseUrl: "http://localhost:9090",
        baseLabelQuery: `{job="cb7"}`,
        targetLabelQuery: `{job="yacpe"}`
    });

    console.log(compare.formatMarkdown(results));
})();
