const core = require('@actions/core');
const github = require('@actions/github');

const compare = require("./compare");


(async function(){
    const config = {
        prometheusBaseUrl: core.getInput("prometheus-base-url"),
        baseLabelQuery: core.getInput("base-label-query"),
        targetLabelQuery: core.getInput("target-label-query"),
    };

    const token = core.getInput("github-token");
    const octokit = github.getOctokit(token);


    const results = await compare.default(config);

    const newComment = await octokit.rest.issues.createComment({
        owner: github.context.repo.owner,
        repo: github.context.repo.repo,
        issue_number: github.context.issue.number,
        // language=Markdown
        body: compare.formatMarkdown(results, github.context.sha)
    });

})();
