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
        body: compare.formatMarkdown(results)
    });

    const me = await octokit.rest.users.getAuthenticated();

    const commentsQuery = await octokit.graphql(
        `query IssuePriorComments($repoName: String!, $repoOwner: String!, $issueNumber: Int!) {
            repository(name: $repoName, owner: $repoOwner) {
                pullRequest(number: $issueNumber) {
                    comments(last: 50) {
                        id
                        databaseId
                        body
                        author {
                            login
                        }
                    }
                }
            }
        }`, {
            repoName: github.context.repo.name,
            repoOwner: github.context.repo.owner,
            issueNumber: github.context.issue.number
        }
    );
    for (const {node} of commentsQuery.repository.pullRequest.comments.edges) {
        if (node.author.login === me.login
            && node.databaseId !== newComment.id
            && node.body.includes("Comparison Results")) {
            await octokit.graphql(
                `mutation HideOldComment($id: ID!) {
                    minimizeComment(input: { subjectId: $id, classifier: OUTDATED }) {
                        minimizedComment {
                            isMinimized
                        }
                    }
                }`
            )
        }
    }
})();
