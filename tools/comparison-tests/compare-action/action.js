/*
 * Copyright 2022 Couchbase, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

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
