<script lang="ts">
	import { responsive } from '$lib/stores';
	import { darkMode } from '$lib/stores';
	import { MenuIcon } from 'lucide-svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import Footer from '$lib/components/Footer.svelte';
	import { goto } from '$app/navigation';
	import { twMerge } from 'tailwind-merge';
	import { onMount } from 'svelte';

	const sectionHeaders = [
		{ id: 'terms-of-service', label: 'Terms of Service' },
		{ id: 'scope-of-the-agreement', label: 'Scope of the Agreement' },
		{ id: 'subscription-to-the-software', label: 'Subscription to the Software' },
		{ id: 'customer-limitations-responsibilities', label: 'Customer Limitations/Responsibilities' },
		{ id: 'data-responsibilities', label: 'Data Responsibilities' },
		{
			id: 'intellectual-property-rights-and-ownership',
			label: 'Intellectual Property Rights And Ownership'
		},
		{ id: 'term-and-termination', label: 'Term and Termination' },
		{ id: 'fees-and-payment', label: 'Fees and Payment' },
		{ id: 'representations-and-warranties', label: 'Representations and Warranties' },
		{ id: 'indemnification', label: 'Indemnification' },
		{
			id: 'use-of-product-information-feedback-and-audit',
			label: 'Use of Product Information, Feedback, and Audit'
		},
		{ id: 'limitations', label: 'Limitations' },
		{ id: 'miscellaneous', label: 'Miscellaneous' },
		{ id: 'appendix-1-definitions', label: 'Appendix 1: Definitions' }
	];

	let selected = $state(sectionHeaders[0].id);

	onMount(() => {
		const handleScroll = () => {
			const viewportHeight = window.innerHeight;
			const scrollPosition = window.scrollY;
			const viewportCenter = scrollPosition + viewportHeight / 2;

			let closestSection = sectionHeaders[0];
			let minDistance = Infinity;

			sectionHeaders.forEach((header) => {
				const element = document.getElementById(header.id);
				if (element) {
					const rect = element.getBoundingClientRect();
					const elementCenter = scrollPosition + rect.top + rect.height / 2;
					const distance = Math.abs(viewportCenter - elementCenter);

					if (distance < minDistance) {
						minDistance = distance;
						closestSection = header;
					}
				}
			});

			selected = closestSection.id;
		};

		handleScroll();

		window.addEventListener('scroll', handleScroll);
		return () => {
			window.removeEventListener('scroll', handleScroll);
		};
	});
</script>

{#snippet navLinks()}
	<a href="https://docs.obot.ai" class="icon-button" rel="external" target="_blank">Docs</a>
	<a href="https://discord.gg/9sSf4UyAMC" class="icon-button" rel="external" target="_blank">
		{#if darkMode.isDark}
			<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
		{:else}
			<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
		{/if}
	</a>
	<a
		href="https://github.com/obot-platform/obot"
		class="icon-button"
		rel="external"
		target="_blank"
	>
		{#if darkMode.isDark}
			<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
		{:else}
			<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
		{/if}
	</a>
{/snippet}

<svelte:head>
	<title>Obot - Terms of Service</title>
</svelte:head>

<div class="relative flex h-dvh w-full flex-col text-black dark:text-white">
	<!-- Header with logo and navigation -->
	<div class="colors-background flex h-16 w-full items-center p-5">
		<div class="relative flex items-end">
			{#if darkMode.isDark}
				<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
			{:else}
				<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
			{/if}
			<div class="ml-1.5 -translate-y-1">
				<span
					class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
				>
					BETA
				</span>
			</div>
		</div>
		<div class="grow"></div>
		<div class="flex items-center gap-4">
			{#if !responsive.isMobile}
				{@render navLinks()}
			{/if}
			<button class="icon-button" onclick={() => goto('/?rd=/')}>Login</button>
			{#if responsive.isMobile}
				<Menu
					slide="left"
					fixed
					classes={{
						dialog:
							'rounded-none h-[calc(100vh-64px)] p-4 left-0 top-[64px] w-full h-full px-4 divide-transparent dark:divide-transparent'
					}}
					title=""
				>
					{#snippet icon()}
						<MenuIcon />
					{/snippet}
					{#snippet body()}
						<div class="flex flex-col gap-2 py-2">
							{@render navLinks()}
						</div>
					{/snippet}
				</Menu>
			{/if}
		</div>
	</div>

	<main
		class="colors-background mx-auto flex w-full max-w-(--breakpoint-2xl) grow flex-col items-center justify-center px-4 pb-12 md:px-12"
	>
		<div class="tos-bg mt-12 flex h-24 w-full items-center rounded-2xl lg:h-40">
			<h1
				class="relative z-10 px-4 text-2xl font-semibold text-white md:px-6 md:text-4xl lg:px-12 lg:text-5xl"
				id={sectionHeaders[0].id}
			>
				Terms of Service
			</h1>
		</div>
		<div class="relative mt-12 flex w-full gap-8">
			<div class="hidden flex-shrink-0 flex-col text-base font-light md:flex md:w-xs lg:w-sm">
				<ul class="sticky top-0 left-0 flex flex-col pt-8 pr-8">
					{#each sectionHeaders as header (header.id)}
						<li
							class={twMerge(
								'border-l-4 border-transparent',
								selected === header.id && 'border-blue-500'
							)}
						>
							<button
								class="flex px-4 py-2 text-left"
								onclick={() => {
									selected = header.id;
									const element = document.getElementById(header.id);
									if (element) {
										window.scrollTo({
											top: element.offsetTop,
											behavior: 'smooth'
										});
										history.pushState(null, '', `#${header.id}`);
									}
								}}
							>
								{header.label}
							</button>
						</li>
					{/each}
				</ul>
			</div>
			<div class="flex grow flex-col gap-4 text-base">
				<p><i>Last Updated: April 21, 2025</i></p>
				<p>
					THIS OBOT.AI SAAS AGREEMENT (“AGREEMENT”) GOVERNS YOUR (“CUSTOMER”) USE OF ACORN LABS,
					INC. (HEREAFTER “ACORN”) OBOT.AI SOFTWARE-AS-A-SERVICE PLATFORM.
				</p>
				<p>
					IF YOU REGISTER FOR A FREE TRIAL OF OUR SOFTWARE, THIS AGREEMENT WILL ALSO GOVERN THAT
					FREE TRIAL.
				</p>
				<p>
					BY ACCEPTING THIS AGREEMENT, EITHER BY CLICKING A BOX INDICATING YOUR ACCEPTANCE OR TAKING
					OTHER AFFIRMATIVE STEPS TO INDICATE THE SAME, INCLUDING OR BY AGREEING AN ORDER FORM
					DURING AN ON-LINE PURCHASE OF SUBSCRIPTIONS THAT REFERENCES THIS AGREEMENT, YOU AGREE TO
					THE TERMS OF THIS AGREEMENT.
				</p>
				<p>
					IF YOU ARE ENTERING INTO THIS AGREEMENT ON BEHALF OF A COMPANY OR OTHER LEGAL ENTITY, YOU
					REPRESENT THAT YOU HAVE THE AUTHORITY TO BIND SUCH ENTITY AND ITS AFFILIATES TO THESE
					TERMS AND CONDITIONS, IN WHICH CASE THE TERM “CUSTOMER” SHALL REFER TO YOU AND/OR SUCH
					ENTITY AND ITS AFFILIATES. IF YOU DO NOT HAVE SUCH AUTHORITY, OR IF YOU DO NOT AGREE WITH
					THESE TERMS AND CONDITIONS, YOU MUST NOT ACCEPT THIS AGREEMENT AND MAY NOT USE THE
					SOFTWARE OR SERVICES.
				</p>
				<p>
					Customer may not access the Software or Services if Customer is a direct competitor of
					Acorn, except with Acorn's prior written consent. In addition, Customer may not access the
					Software or Services for purposes of monitoring their availability, performance or
					functionality.
				</p>
				<p>
					Customer's use of the Software or Services constitutes Customer's agreement to these
					terms. It is effective between Customer and Acorn as of the date Customer signs or
					consents to an Order Form or other applicable ordering document, or first uses the
					Software or Services, whichever is earlier (the "Effective Date").
				</p>

				<p>
					NOW THEREFORE, the Parties enter into this Agreement for the provision of Software or
					Services to Customer.
				</p>

				<ol>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[1].id}>Scope of the Agreement</h2>
						<ol>
							<li>
								<b>Ordering.</b> Any Software or Services purchased by Customer shall be governed by
								an Order Form. Professional Services will be governed by a separate written agreement
								or an Order Form.
							</li>
							<li>
								<b>Structure.</b> The Agreement also incorporates the following components: (1) the applicable
								Order Form entered into by the Customer, and (2) and any applicable SOW for Professional
								Services. Unless otherwise specified in an Order Form, terms defined in this Agreement
								shall have the same meaning when used in any other document made part of this Agreement.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[2].id}>
							Subscription to the Software
						</h2>
						<ol>
							<li>
								<b>Usage Limits.</b> Subscriptions for the Software are limited to the quantities specified
								in the applicable Software Order Form. Each Subscription refers to an individual User.
								The Software may not be accessed by more Users than reflected by the number of Subscriptions
								in a Software Order. A Subscription may be reassigned to a different User by Customer.
								If Customer exceeds its Subscriptions to the Software, Customer will, upon Acorn's request,
								promptly execute a Software Order for sufficient additional Subscriptions to comply with
								the Agreement. Customer will pay Acorn's invoice for the excess usage according to the
								Agreement.
							</li>
							<li>
								<b>Beta Software.</b> Acorn may invite Customer to try Software that is not generally
								available to customers (“Beta Software”) at no charge. Customer is under no obligation
								to use Beta Software. Beta Software will be clearly designated. Beta Software is: (a)
								for evaluation purposes only and not for production use, (b) are not considered "Software"
								under the Agreement, and (c) are not included in any support that may be offered by Acorn.
								Acorn may discontinue Beta Software at any time. Beta Software is provided "as-is" without
								warranty, and notwithstanding Section 12, Acorn will have no liability for any claim
								arising from Customer's, its Affiliates', or Users' use of Beta Software.
							</li>
							<li>
								<b>AI Features.</b> The Software employs artificial intelligence, machine learning, or
								similar technologies through the Software' processing of Customer Data (the "AI Features").
								Customer or its Authorized Users may provide input, including Customer Data, for use
								with the AI Features ("AI Input") and receive output generated and returned by the AI
								Features based on the AI Input ("AI Output"). Other customers providing similar AI Input
								to the Al Features may receive the same or similar AI Output. Customer acknowledges and
								agrees that Customer is responsible for reviewing and validating AI Output for its needs
								and technical environment before electing to use AI Output. Customer agrees to comply
								with any applicable AI Feature restrictions described in the Documentation. NOTWITHSTANDING
								ANY CONTRARY PROVISION HEREIN, ACORN DOES NOT REPRESENT OR WARRANT THAT THE AI OUTPUT
								WILL BE ACCURATE, COMPLETE, ERROR-FREE, OR FIT FOR A PARTICULAR PURPOSE.
							</li>
							<li>
								<b>Connected Applications.</b> The Software contains features designed to interoperate
								with Connected Applications. To use such features, Customer or its Users may be required
								to obtain access to such Connected Applications from their providers and grant the Software
								access to Customer's or its Users' account(s) on such Connected Applications. If Customer
								uses a Connected Application with the Software, Customer grants Acorn permission to allow
								the Connected Application and its provider to access Customer Data solely as required
								for the interoperability of that Connected Application with the Software. Disclaimer:
								Acorn provides interoperability with Connected Applications as a courtesy, on an as-is
								basis, and not part of the Subscription. Acorn makes no warranty or guarantee as to the
								interoperability or availability of any Connected Applications and the Customer's use
								of any such Connected Applications is wholly at Customer's own risk. Acorn may terminate
								interoperability with Connected Applications at any time in Acorn's sole discretion,
								after providing Customer commercially reasonable notice (except in the case where the
								Connected Application poses a security risk to the Software). Any acquisition by Customer
								of Connected Applications, and any exchange of Customer Data between Customer and any
								Connected Application provider, product, or Software, is solely between Customer and
								the applicable Connected Application provider. Acorn does not warrant or support Connected
								Applications. Acorn is not responsible for any disclosure, modification or deletion of
								Customer Data resulting from access by any Connected Application or its provider.
							</li>
							<li>
								<b>Public AI Agents; Disclaimer of Warranties.</b> The Software may display or make available
								AI agents, models, or automation workflows that have been created and published by third
								parties ("Public AI Agents"). These Public AI Agents are provided solely for convenience,
								informational purposes, or optional use, and are not created, reviewed, or endorsed by
								Acorn. No Warranty. Public AI Agents are provided "as-is" and "as available", without
								any warranties or guarantees of any kind, whether express, implied, statutory, or otherwise,
								including but not limited to any warranties of accuracy, performance, fitness for a particular
								purpose, merchantability, or non-infringement. Customer Responsibility. Customer is solely
								responsible for its decision to use, rely on, or deploy any Public AI Agent. Acorn shall
								have no liability for any damage, loss, or harm (including loss of data or system failure)
								resulting from Customer's use of any Public AI Agent, whether to its systems or to third
								parties. Customer agrees to use appropriate caution and perform its own testing, validation,
								and review before implementing any Public AI Agent in a production or critical environment.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[3].id}>
							Customer Limitations/Responsibilities
						</h2>
						<ol>
							<li>
								<b>Limitations.</b> Customer shall not: (a) resell, sublicense, rent, loan, lease,
								time share or otherwise make the Software available to any party not authorized to
								use the Software under the Agreement or an applicable Software Order; (b) modify,
								adapt, alter, translate, copy, or create derivative works based on the Software; (c)
								reverse-engineer, decompile, disassemble, or attempt to derive the source code for
								the Software (unless such right is granted by applicable law and then only to the
								minimum extent required by law); (d) access the Software in order to: (i) build a
								competitive product or service; or (ii) copy any ideas, features, functions or
								graphics of the Software; (e) merge or use the Software with any software or
								hardware for which they were not intended (as described in the Documentation); (f)
								allow Users to share access credentials;
								<b
									>(g) use the Software for unlawful purposes or to store unlawful material,
									including but not limited to, for the purpose of exploiting, harming, or
									attempting to exploit or harm minors in any way by exposing them to inappropriate
									content or otherwise; (h) to transmit, or procure the sending of, any advertising
									or promotional material, including any "junk mail", "chain letter," "spam," or any
									other similar solicitation;</b
								> (i) use the Software to send or store material containing software viruses, worms,
								Trojan horses or other harmful computer code, files, scripts, or agents; (j) disrupt
								the integrity or performance of the Software; (k) remove, alter, or obscure in any way
								the proprietary rights notices (including copyright, patent, and trademark notices and
								symbols) of Acorn or its suppliers contained on or within any copies of the Software,
								(l) bypass any security measure or access control measure of the Software, (m) use the
								Software other than as described in the Documentation, (n) perform or disclose any benchmarking
								or testing of the Software itself or of the security environment or associated infrastructure
								without Acorn's prior written consent. Acorn may, without limiting its other rights and
								remedies, suspend Customer's and/or applicable Users' access to the Software at any time
								if: (i) required by applicable law, (ii) Customer or any User is in violation of the
								terms of this Agreement, or (iii) Customer's or a User's use disrupts the integrity or
								operation of the Software or interferes with use of the Software by others. Acorn will
								use reasonable efforts to notify Customer prior to any suspension, unless prohibited
								by applicable law or court order, and Acorn will promptly restore Customer's access to
								the Software upon resolution of any violation under this section. If Acorn is notified
								that any Customer Data violates applicable law or third-party rights, Acorn may so notify
								Customer and in such event Customer will promptly remove such Customer Data from the
								Software. If Customer does not take the required action, Acorn may disable the applicable
								Customer Data until the potential violation is resolved.
							</li>
							<li>
								<b>Customer Responsibilities.</b> Customer will: (a) at all times remain responsible
								for Users' compliance with the Agreement and will promptly notify Acorn of any unauthorized
								access to the Software arising from a compromise or misuse of Customer's or its User's
								access credentials, (b) use the Software only in accordance with the Documentation, applicable
								laws, and government regulations, (c) comply with terms of service of any Connected Applications
								Customer uses in conjunction with the Software, and (d) remain responsible for any action
								in violation of the Agreement by Customer's Affiliates or Users.
							</li>
							<li>
								<b>AI Agent Responsibilities/Indemnity.</b> Customer is solely responsible for the actions,
								decisions, and outputs of any AI agents, models, or automated processes that it creates,
								configures, deploys, or instructs using the Software. This includes any interactions
								such AI Agents initiate with third parties, third-party systems or software, any data
								they process or generate, and any consequences arising from their behavior or performance.
								Customer agrees that it shall: (a) ensure all AI Agents comply with applicable laws,
								regulations, and third-party rights, including but not limited to applicable data privacy
								and processing regulations, (b) not use the Software to create AI Agents that engage
								in deceptive, harmful, or unlawful conduct, and (c) monitor and control the behavior
								of all AI Agents it deploys and implement appropriate safeguards to prevent misuse.
							</li>
							<li>
								<b>Customer AI Agent Indemnity.</b> Customer agrees to indemnify, defend, and hold harmless
								Acorn, its affiliates, and their respective officers, directors, employees, and agents
								from and against any and all claims, liabilities, damages, losses, and expenses (including
								reasonable attorneys' fees) arising out of or in connection with any AI Agent created
								or operated by Customer, or on Customer's behalf.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[4].id}>Data Responsibilities</h2>
						<ol>
							<li>
								<b>Compliance With Applicable Laws.</b> Customer is exclusively responsible for: (a)
								determining what data Customer submits to the Software, (b) for obtaining all necessary
								consent and permissions for submission of Customer Data and related data processing instructions
								to Acorn, (c) for the accuracy, quality and legality of Customer Data, and (d) Customer's
								compliance with applicable data privacy and protection regulations. Customer shall ensure
								that it is entitled to transfer the relevant Customer Data to Acorn so that Acorn and
								its service providers may lawfully use, process, and transfer the Customer Data in accordance
								with this Agreement on Customer's behalf. No rights to the Customer Data are granted
								to Acorn hereunder other than as expressly set forth in this Agreement.
							</li>
							<li>
								<b>Excluded Data.</b> Customer shall not provide Acorn with any Customer Data that is
								subject to heightened security requirements by law, regulation or contract (examples
								include but are not limited to the Gramm–Leach–Bliley Act (GLBA), Health Insurance and
								Portability and Accountability Act (HIPAA), Family Educational Rights and Privacy Act
								(FERPA), the Child's Online Privacy Protection Act (COPPA), the standards promulgated
								by the PCI Security Standards Council (PCI-DSS), and their international equivalents
								(such Customer Data collectively, "Excluded Data"). Acorn shall have no responsibility
								or liability for Excluded Data.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[5].id}>
							Intellectual Property Rights And Ownership
						</h2>
						<ol>
							<li>
								<b>Reservation of Rights.</b> Access to the Software is sold on a subscription basis.
								Except for the limited rights expressly granted to Customer hereunder, Acorn reserves
								all rights, title, and interest in and to the Software, the underlying software, the
								Acorn Materials and any and all improvements (including any arising from Customer's feedback),
								modifications and updates thereto, including without limitation all related intellectual
								property rights inherent therein. Where Customer purchases Professional Services hereunder,
								Acorn grants to Customer a non-sublicensable, non-exclusive license to use any materials
								provided by Acorn as a result of the Professional Services (the "Acorn Materials") solely
								in conjunction with Customer's authorized use of the Software and in accordance with
								this Agreement. No rights are granted to Customer hereunder other than as expressly set
								forth in this Agreement. Nothing in this Agreement will impair Acorn's right to develop,
								acquire, license, market, promote or distribute products, software or technologies that
								perform the same or similar functions as, or otherwise compete with, any products, software
								or technologies that Customer may develop, produce, market, or distribute.
							</li>
							<li>
								<b>Ownership and Processing of Customer Data.</b> Customer and/or its licensors shall
								retain all right, title and interest in all Customer Data stored in the Software, including
								any revisions, updates or other changes made to that Customer Data. Customer grants Acorn
								a nonexclusive, worldwide, royalty-free right to reproduce, display, adapt, modify, transmit,
								distribute and otherwise use the Customer Data: (a) solely for the purpose of providing
								the Software and Professional Services under this Agreement; (b) to prevent or address
								technical or security issues and resolve support requests; (c) at Customer's direction
								or request, enable integrations between Customer's Connected Applications and the Software;
								and (d) as otherwise required by applicable law.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[6].id}>Term and Termination</h2>
						<ol>
							<li>
								<b>Termination by Customer</b> This Agreement shall commence on the date the Customer
								subscribes to the Software and shall continue on a month-to-month basis. The Customer
								may terminate this Agreement at any time through their account settings in the Software,
								with such termination becoming effective immediately. The Customer acknowledges that
								no refunds will be provided for the remainder of the subscription term in which they
								terminate. Acorn shall not be liable for any termination or suspension of Customer's
								subscription in the event the Customer's payment method is expired, deleted, or otherwise
								fails to process a transaction.
							</li>
							<li>
								<b>Termination by Acorn.</b> Acorn may terminate the Customer's subscription to the Software
								at any time, for any reason or no reason, by providing the Customer with thirty (30)
								days' prior written notice. Additionally, Acorn may immediately terminate the Customer's
								subscription for breach of this Agreement. Acorn may, at its sole discretion, provide
								the Customer with an opportunity to cure such breach within the notice period (if applicable).
								If the breach is not cured within the specified period, termination shall become effective
								at the end of the notice period.
							</li>
							<li>
								<b>Effect of Termination; Survival.</b> Upon termination of Customer's subscription Acorn's
								obligation to provide the Software, and Customer's right to access or use the Software,
								will terminate. Acorn will not issue any refunds for any pre-paid but unused Fees. Sections
								3, 5, 6, 7, 8-10, and 12-14 will survive the termination of this Agreement including
								any other term which by its nature and purpose should also survive.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[7].id}>Fees and Payment</h2>
						<ol>
							<li>
								<b>Payment of Fees.</b> At the election of Acorn, Customer agrees to pay the Subscription
								fees for the Software either: (i) on a month-to-month, or (ii) on a usage basis, measured
								by AI tokens purchased by Customer ("Fees"). Customer shall pay Fees via credit card,
								or other method ("Payment Method") accepted by Acorn. Subscription Fees shall be charged
								to the Customer's Payment Method on the first day of each monthly subscription period,
								or when available AI Tokens are exhausted. Customer shall ensure that the Payment Method
								information provided is accurate and up-to-date. In the event of a payment failure due
								to expired or invalid Payment Method information, Customer shall update the payment information
								promptly to avoid service interruption. If payment is not received by Acorn within ten
								(10) days after the due date, Acorn may suspend access to the Software until payment
								is made in full. Acorn reserves the right to charge interest on any overdue amounts at
								a rate of 1.5% per month or the maximum rate permitted by law, whichever is lower, from
								the due date until the date of payment. Customer may dispute any charges by providing
								written notice to Acorn within thirty (30) days of the charge, detailing the nature of
								the dispute. Acorn and Customer shall work in good faith to resolve any such disputes
								promptly. Amounts not disputed within this period shall be deemed accepted by Customer.
								Customer must pay the Fees without withholding or deduction in U.S. Dollars. All Fees
								and other amounts paid under the Agreement are non-refundable.
							</li>
							<li>
								<b>Taxes.</b> The Fees set forth in any Order Form are exclusive of, and Customer is
								liable for and will pay, all taxes, including any value added tax and goods and services
								tax or any similar tax imposed on or measured by this Agreement.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[8].id}>
							Represenations and Warranties
						</h2>
						<ol>
							<li>
								<b>Customer Warranty.</b> Customer represents and warrants that: (a) it has the authority
								to enter into this Agreement, (b) its use of Software or Services will comply with all
								applicable laws, and it will not use the Software or Services for any illegal activity,
								and (c) it has the right to upload and or distribute Customer Data through the Software.
							</li>
							<li>
								TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW THE ACORN SOFTWARE AND SERVICES
								ARE PROVIDED "AS IS" AND WITHOUT ANY REPRESENTATIONS OR WARRANTIES EXPRESS OR
								IMPLIED, AND ACORN DISCLAIMS ALL SUCH REPRESENTATIONS AND WARRANTIES, INCLUDING THE
								IMPLIED WARRANTIES OF MERCHANTABILITY, NON-INFRINGEMENT, AND FITNESS FOR A
								PARTICULAR PURPOSE, AND ANY WARRANTIES IMPLIED BY THE COURSE OF DEALING OR USAGE OF
								TRADE. ACORN AND ITS SUPPLIERS DO NOT REPRESENT OR WARRANT THAT THE ACORN SOFTWARE
								AND SERVICES WILL BE UNINTERRUPTED, SECURE, ERROR FREE, ACCURATE OR COMPLETE OR
								COMPLY WITH REGULATORY REQUIREMENTS, OR THAT ACORN WILL CORRECT ALL ERRORS. IN THE
								EVENT OF A BREACH OF THE WARRANTIES SET FORTH IN SECTION 9, CLIENT'S EXCLUSIVE
								REMEDY, AND ACORN'S ENTIRE LIABILITY, WILL BE THE RE-PERFORMANCE, OR REDELIVERY OF
								THE DEFICIENT ACORN SOFTWARE OR SERVICE, OR IF ACORN CANNOT SUBSTANTIALLY CORRECT A
								BREACH IN A COMMERCIALLY REASONABLE MANNER, TERMINATION OF THE RELEVANT ACORN
								SOFTWARE OR SERVICE, IN WHICH CASE CLIENT MAY RECEIVE A PRO RATA REFUND OF THE
								PREPAID BUT UNUSED FEES PAID FOR THE DEFICIENT ACORN PRODUCT OR SERVICE AS OF THE
								EFFECTIVE DATE OF TERMINATION.
							</li>
							<li>
								The Software and Services have not been tested in all situations under which they
								may be used. Acorn will not be liable for the results obtained through use of the
								Software or Services and Customer is solely responsible for determining appropriate
								uses for the Acorn Software and Services and for all results of such use. For
								example, Acorn Software and Services are not specifically designed, manufactured or
								intended for use in (a) the design, planning, construction, maintenance, control, or
								direct operation of nuclear facilities, (b) aircraft control, navigation, or
								communication systems (c) weapons systems, (d) direct life support systems (e) or
								other similar hazardous environments.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[9].id}>Indemnification</h2>
						<ol>
							<li>
								<b>Imdemnification By Customer.</b> If a third party initiates or threatens legal action
								against Acorn for processing Customer Data uploaded into the Software by Customer or
								Users, or for a claim relating to Customer's, or a User's, breach of its obligations
								under the Agreement, (a "Claim"), then Customer will promptly indemnify, defend, and
								hold harmless Acorn and its officers, directors, employees, agents, successors, and assigns
								(each, a "Acorn Indemnitee") from and against any and all losses, liabilities, damages,
								costs, and expenses, including reasonable attorneys' fees and court costs, incurred by
								any Acorn Indemnitee in connection with any third-party claim, action, or proceeding
								arising out of such Claim. Acorn shall have the right to approve any settlement of a
								Claim.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[10].id}>
							Use of Product Information, Feedback, and Audit
						</h2>
						<ol>
							<li>
								<b>Use of Product or Services Information.</b> Acorn may collect and use for any purpose
								aggregated anonymous benchmark data about Customer's use of the Software or Services.
								Nothing in this Agreement will limit Acorn from providing software, materials, or services
								for itself or other clients, irrespective of the possible similarity of such software,
								materials or services to those that might be delivered to Customer. The terms of Section
								11 will not prohibit or restrict either party's right to develop, use or market products
								or services similar to or competitive with the other party; provided, however, that neither
								party is relieved of its obligations under this Agreement.
							</li>
							<li>
								<b>Feedback.</b> If Customer chooses to voluntarily provide any Feedback to Acorn regarding
								Acorn Software or Services, Acorn may use such Feedback for any purpose, including incorporating
								the Feedback into, or using the Feedback to develop and improve Acorn Software and other
								Acorn offerings without attribution or compensation. Customer grants Acorn a perpetual
								and irrevocable license to use all Feedback for any purpose. Customer agrees to provide
								Feedback to Acorn only in compliance with applicable laws and Customer represents that
								it has the authority to provide the Feedback and that Feedback will not include proprietary
								information of a third party. Acorn acknowledges and agrees that any feedback provided
								by the client under this agreement is on an "as is" basis, without any warranty of any
								kind.
							</li>
						</ol>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[11].id}>Limitations</h2>
						<ol>
							<li>
								<b>DISCLAIMER OF DAMAGES.</b> TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, NEITHER
								ACORN, NOR ITS AFFILIATES, WILL BE LIABLE FOR ANY INCIDENTAL, CONSEQUENTIAL, SPECIAL,
								INDIRECT, EXEMPLARY OR PUNITIVE DAMAGES, OR FOR ANY DAMAGES FOR LOST OR DAMAGED DATA,
								LOST PROFITS, LOST SAVINGS OR BUSINESS OR SERVICE INTERRUPTION, EVEN IF SUCH PARTY WAS
								ADVISED OF THE POSSIBILITY OF SUCH DAMAGES, AND REGARDLESS OF THE FAILURE OF ESSENTIAL
								PURPOSE OF ANY LIMITED REMEDY.
							</li>
							<li>
								<b>LIMITATION OF LIABILITY.</b> TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, NEITHER
								ACORN NOR ITS AFFILIATES' TOTAL AND AGGREGATE LIABILITY WITH RESPECT TO ANY CLAIM RELATING
								TO OR ARISING OUT OF THIS AGREEMENT WILL EXCEED THE FEES RECEIVED BY ACORN WITH RESPECT
								TO THE PARTICULAR ACORN SOFTWARE OR SERVICE GIVING RISE TO LIABILITY UNDER THE MOST APPLICABLE
								ORDERING DOCUMENT DURING THE THREE (3) MONTHS IMMEDIATELY PRECEDING THE FIRST EVENT GIVING
								RISE TO SUCH CLAIM. THIS LIMITATION APPLIES REGARDLESS OF THE NATURE OF THE CLAIM, WHETHER
								CONTRACT, TORT (INCLUDING NEGLIGENCE), STATUTE OR OTHER LEGAL THEORY. THESE LIMITATIONS
								DO NOT LIMIT CLAIMS OF BODILY INJURY (INCLUDING DEATH) AND DAMAGE TO REAL OR TANGIBLE
								PERSONAL PROPERTY CAUSED BY THE NEGLIGENCE OF A PARTY OR ITS AFFILIATES.
							</li>
						</ol>
					</li>
					<li>
						<p>
							<b>Governing Law and Claims.</b> The Agreement, and any claim, controversy or dispute related
							to the Agreement, are governed by and construed in accordance with the laws of the State
							of California without giving effect to any conflicts of laws provisions. To the extent
							permissible, the United Nations Convention on Contracts for the International Sale of Goods
							will not apply, even if adopted as part of the laws of the State of California. Any claim,
							suit, action or proceeding arising out of or relating to this Agreement or its subject
							matter will be brought exclusively in the state or federal courts of Santa Clara County,
							California, and each party irrevocably submits to the exclusive jurisdiction and venue.
							No claim or action, regardless of form, arising out of this Agreement may be brought by
							either party more than one (1) year after the earlier of the following: (a) the expiration
							of all Subscriptions or other ordering documents, (b) the termination of this Agreement,
							or (c) the time a party first became aware, or reasonably should have been aware, of the
							basis for the claim. To the fullest extent permitted, each party waives the right to trial
							by jury in any legal proceeding arising out of or relating to this Agreement or the transactions
							contemplated hereby.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[12].id}>Miscellaneous</h2>
						<ol>
							<li>
								<b>Export.</b> Customer will not provide to Acorn any data or engage Acorn in any activity,
								in each case, that could constitute the development of a "defense article" or provision
								of a "defense service" to Customer, as these terms are defined in Section 120 of the
								International Traffic in Arms Regulations (ITAR). In addition, Customer will not, and
								will not allow third parties under Customer's control, (i) to provide Acorn with Customer
								Data that requires an export license under applicable export control laws or (ii) to
								process or store any Customer Data that is subject to the ITAR. If Customer breaches
								(or Acorn believes Customer has breached) this Section or the export provisions of an
								end user license agreement for any software or Acorn is prohibited by law or otherwise
								restricted from providing Software or Services to Customer, Acorn may terminate this
								Agreement and/or the applicable Order Form. Customer shall comply with all applicable
								U.S. export control laws.
							</li>
							<li>
								<b>Notices.</b> Notices must be in English, in writing, and transmitted to each
								party by the regular communication functionality provided within the Software, and
								will be deemed given upon receipt. Any notice from Customer to Acorn must include a
								copy sent to <a href="mailto:legal@acorn.io">legal@acorn.io</a>.
							</li>
							<li>
								<b>Waiver.</b> A waiver by a party under this Agreement is only valid if in writing and
								signed by an authorized representative of such party. A delay or failure of a party to
								exercise any rights under this Agreement will not constitute or be deemed a waiver or
								forfeiture of such rights.
							</li>
							<li>
								<b>Independent Contractors.</b> The parties are independent contractors and nothing in
								this Agreement creates an employment, partnership or agency relationship between the
								parties or any Affiliate. Each party is solely responsible for supervision, control and
								payment of its personnel and contractors.
							</li>
							<li>
								<b>Third Party Beneficiaries.</b> This Agreement is binding on the parties to this Agreement
								and, other than as expressly provided in the Agreement, nothing in this Agreement grants
								any other person or entity any right, benefit or remedy.
							</li>
							<li>
								<b>Force Majeure.</b> Neither party is responsible for nonperformance or delay in performance
								of its obligations (other than payment of Fees) due to causes beyond its reasonable control.
								If the period of non-performance of one party exceeds 30 calendar days from receipt of
								notice of the force majeure event, the other party may, by giving written notice, terminate
								this Agreement.
							</li>
							<li>
								<b>Complete Agreement and Order of Precedence.</b> The Agreement represents the complete
								agreement between the parties with respect to its subject matter and supersedes all prior
								and contemporaneous agreements and proposals, whether written or oral, with respect to
								such subject matter, including any prior confidentiality agreements entered into by the
								parties. Any terms contained in any other documentation that Customer delivers to Acorn,
								including any purchase order or other order-related document (other than an Order Form),
								are void and will not become part of the Agreement or otherwise bind the parties. To
								the extent of any conflict or ambiguity between the terms and conditions of the Agreement
								and any other agreement applicable among the parties under the Agreement, the following
								order of precedence will apply: (1) any fully executed Statement of Work among the parties;
								(2) the Agreement; (3) all other documents and policies applicable between the parties.
							</li>
							<li>
								<b>Severable.</b> If any provision of this Agreement is held by a court of competent
								jurisdiction to be invalid or unenforceable, the remaining provisions of this Agreement
								will remain in effect to the greatest extent permitted by law.
							</li>
						</ol>
					</li>
				</ol>

				<h2 class="text-lg font-semibold" id={sectionHeaders[13].id}>Appendix 1: Definitions</h2>
				<ul class="ml-4 flex list-disc flex-col gap-4">
					<li>
						<b>"AI Agent"</b> means any software-based system created, configured, or operated through
						the Software that functions with a degree of autonomy to perform tasks, make decisions, generate
						outputs, or interact with systems or users, without requiring constant human input or oversight.
					</li>

					<li>
						<b>AI Token</b> means Acorn's billing system where Customers are charged based on the number
						of tokens (i.e. units of text) they input and receive.
					</li>

					<li>
						<b>Affiliate</b> means any person or entity directly or indirectly controlling, controlled
						by or under common control with a Party as of or after the Effective Date, for so long as
						that relationship is in effect (including affiliates subsequently established by acquisition,
						merger or otherwise).
					</li>

					<li>
						<b>Authorized User or User</b> means: (a) in the case of an individual accepting this Agreement
						on such individual's own behalf, such individual; or (b) an employee or authorized third-party
						of Customer, who has been authorized by Customer to use the Service in accordance with the
						terms and conditions of this Agreement and has been allocated user credentials.
					</li>

					<li>
						<b>Connected Application</b> means Customer's or a third party's web-based or other software
						application interoperates with the Software.
					</li>

					<li>
						<b>Customer Data</b> means any electronic data or materials provided or submitted by or for
						Customer to or through the Software.
					</li>

					<li>
						<b>Fees</b> are the amounts to be paid by Customer to Acorn for the Acorn Software or Services.
					</li>

					<li>
						<b>Order Form</b> means Acorn's standard ordering document or online form used to order Acorn
						Software or Services.
					</li>

					<li><b>Product(s) or Acorn Product(s)</b> means Acorn Software.</li>

					<li>
						<b>Professional Services</b> means Training Services or other services Customer agrees to
						purchase as described in a fully executed Statement of Work.
					</li>

					<li>
						<b>Software</b> means Acorn, Inc.'s obot.ai branded software-as-service platform located
						at obot.ai, including any applicable subdomains of each, and any web browser extension obtained
						for use thereon.
					</li>

					<li>
						<b>Statement of Work ("SOW")</b> means the documentation of an order for Acorn Professional
						Services consisting of a description of the services to be performed and other associated
						information such as the term of these services.
					</li>

					<li>
						<b>Subscription</b> means access to the Software during the Subscription Term. Each Subscription
						is specific to a unique Authorized User and under no circumstance may an Authorized User
						Subscription be transferred to, shared among or used by different Authorized Users.
					</li>

					<li>
						<b>Subscription Term</b> means either: (i) the period during which Customer is authorized
						to use the Software measured on a month-to-month basis, or (ii) the period during which Customer
						has available AI Tokens for use, each (i) and (ii) as specified on an Order Form.
					</li>

					<li>
						<b>Supplier</b> means a third party that provides service(s) to Acorn in order for Acorn
						to offer Software or Services to its customers and/or business partners.
					</li>

					<li>
						<b>Taxes</b> means any form of taxation of whatever nature and by whatever authority imposed,
						including any interest, surcharges or penalties, arising from or relating to this Agreement
						or any Acorn Software, other than taxes based on the net income of Acorn.
					</li>

					<li>
						<b>Training Services</b> are Acorn's training courses delivered onsite or remotely as the
						Parties agree in an applicable Order Form.
					</li>
				</ul>
			</div>
		</div>
	</main>
	<Footer />
</div>

<style lang="postcss">
	ol {
		list-style-type: none;
		counter-reset: item;
		margin: 0;
		padding: 0 0 1rem 0;
	}

	ol > li {
		display: table;
		counter-increment: item;
		margin-bottom: 1rem;
	}

	ol > li:before {
		content: counters(item, '.') '. ';
		display: table-cell;
		padding-right: 1rem;
	}

	li ol > li {
		margin: 0;
		padding-bottom: 1rem;
	}

	li ol > li:before {
		content: counters(item, '.') ' ';
	}

	ol > li > ol {
		padding-top: 1rem;
	}

	.tos-bg {
		&:after {
			content: '';
			width: 1440px;
			height: 160px;
			position: absolute;
			background-image: radial-gradient(
				circle at 85% 50%,
				#557de5 0%,
				#557de5 10%,
				#4e76dc 10%,
				#4e76dc 18%,
				#5074d4 18%,
				#5074d4 30%,
				#4d70d0 30%,
				#496ac6 50%,
				#4667c4 50%,
				#4060bc 75%,
				#3f5eb8 75%
			);
			background-size: 100%;
			background-position: right;
			top: 0;
			right: 0;
		}

		position: relative;
		overflow: hidden;

		&::before {
			content: '';
			position: absolute;
			background-image: url('/user/images/obot-icon-white.svg');
			width: 10rem;
			height: 10rem;
			background-size: 100%;
			background-position: center;
			background-repeat: no-repeat;
			top: -0.75rem;
			right: 1rem;
			z-index: 1;
		}
	}

	@media (min-width: 1024px) {
		.tos-bg {
			&::before {
				width: 20rem;
				height: 20rem;
				top: -2rem;
				right: 2rem;
			}
		}
	}
</style>
