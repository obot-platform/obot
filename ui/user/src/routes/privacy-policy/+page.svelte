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
		{ label: 'Privacy Policy', id: 'privacy-policy' },
		{ label: 'Scope of This Notice', id: 'scope-of-this-notice' },
		{ label: 'Personal Data We May Collect', id: 'personal-data-we-may-collect' },
		{ label: 'Use of Personal Data', id: 'use-of-personal-data' },
		{ label: 'Sharing Personal Data', id: 'sharing-personal-data' },
		{ label: 'Legal Basis for Processing', id: 'legal-basis-for-processing' },
		{ label: 'Cookies And Similar Technologies', id: 'cookies-and-similar-technologies' },
		{ label: 'Protections of Personal Data', id: 'protections-of-personal-data' },
		{ label: 'How Long We Retain Personal Data', id: 'how-long-we-retain-personal-data' },
		{ label: 'Children’s Privacy', id: 'childrens-privacy' },
		{
			label: 'Your Rights and Controlling Your Personal Information',
			id: 'your-rights-and-controlling-your-personal-information'
		},
		{ label: 'Contact or Complain Information', id: 'contact-or-complain-information' },
		{ label: 'Changes to This Notice', id: 'changes-to-this-notice' }
	];

	let selected = $state(sectionHeaders[0].id);

	onMount(() => {
		const handleScroll = () => {
			const viewportHeight = window.innerHeight;
			const scrollPosition = window.scrollY;
			const viewportCenter = scrollPosition + viewportHeight / 2;

			let closestSection = sectionHeaders[0];
			let minDistance = Infinity;

			const documentHeight = document.documentElement.scrollHeight;
			const scrollBottom = scrollPosition + viewportHeight;

			// If at the top of the page, select first section
			if (scrollPosition === 0) {
				selected = sectionHeaders[0].id;
				return;
			}

			// If at the bottom of the page, select last section
			if (scrollBottom >= documentHeight) {
				selected = sectionHeaders[sectionHeaders.length - 1].id;
				return;
			}

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
	<title>Obot - Privacy Policy</title>
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
				Privacy Policy
			</h1>
		</div>
		<div class="relative mt-12 flex w-full gap-8">
			<div class="hidden flex-shrink-0 flex-col text-base font-light md:flex md:w-xs lg:w-sm">
				<ul class="sticky top-0 left-0 flex flex-col pt-8 pr-8">
					{#each sectionHeaders as header}
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
			<div class="doc flex grow flex-col gap-4 text-base">
				<p>
					At Acorn we are committed to protecting personal information. In this Privacy Notice
					(“Notice”), Acorn Labs, Inc. and its affiliated entities (“Acorn”, “we,” or “us”) set out
					how we, as data controller, may collect, create, share and use personal information
					relating to identifiable individuals (“Personal Data”) and provide you with information
					regarding your Personal Data rights and choices.
				</p>
				<p>
					If you, or the company you represent, have entered into the Acorn Obot.ai SaaS Agreement
					(https://www.obot.ai/terms), or a negotiated a custom agreement, to use the
					Software-as-a-Service platform located at https://obot.ai (the “Services”), this
					Notice is made part of, and integrated into, that document (each an “Acorn Obot.ai SaaS
					Agreement”). By continuing to use our Sites or the Services, you agree to this Privacy
					Notice. Capitalized terms not defined herein have the meaning set for in Acorn Obot.ai
					SaaS Agreement.
				</p>

				<ol>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[1].id}>SCOPE OF THIS NOTICE</h2>
						<p>
							This Notice describes Acorn’s privacy practices for Personal Data collected by Acorn
							websites (www.Acorn.com and www.obot.ai , including any and all of subdomains,
							together the “Site(s)”) and the Services, as well as other activities where you may
							share personal data with Acorn.
						</p>
						<p>
							Please read this Notice in full. By using our Sites, using the Services, or
							participating in other activities with Acorn described below, you consent to the
							collection, creation, sharing and use of information as described in this Notice,
							where consent is required by relevant law.
						</p>
						<p>
							Unless specifically identified, references to Services will include both the operation
							of Sites as well as the Services.
						</p>
						<p>
							Acorn may also operate a public forum on certain Acorn social media pages and on our
							websites (“Forums”). The purpose of Forums is to discuss our products and Services.
							Please note that any Personal Data you choose to post in a Forum may be read or used
							by other visitors (for example, to send you unsolicited messages). Acorn Forum users
							should not upload sensitive or confidential content on this public forum.
						</p>
						<p>
							As a convenience to visitors to our Sites, we may also provide links to other websites
							that are not governed by this Notice. These linked websites are not under the control
							of Acorn and we are not responsible for the content on such websites or the
							protection/privacy of any information which you provide while visiting such websites.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[2].id}>
							PERSONAL DATA WE MAY COLLECT
						</h2>
						<p>
							The Personal Data Acorn may collect about you will depend on your
							interaction/relationship with us. It may include:
						</p>
						<ul class="list-disc pl-4">
							<li>account information you give us about yourself, such as contact information;</li>
							<li>
								number if you browse our Sites, information systems data relating to your
								interaction with our Sites, such as location (e.g., IP address), cookie related
								information (see section 6 below), usage (e.g., mouse clicks, page visits, feature
								usage, view time, downloads) and other information, such as language preference;
							</li>
							<li>
								location information we collect which is automatically generated when devices,
								products and services interact with cell towers and Wi- Fi routers. Location can
								also be generated by Bluetooth services, network devices and other tech, including
								GPS satellites;
							</li>
							<li>
								if you complete a webform on our Sites (e.g., to submit a request, lead, or review),
								attend an Acorn event (online or in-person), or otherwise send this data to Acorn
								(e.g., by email, telephone, exchange of business cards), identifying/contact
								information such as your name, job title, work email, organization/employer name,
								work telephone, and location information such as work address;
							</li>
							<li>
								content you provide through our products. As part of the Services, we collect and
								store the content you post, send, receive, and share through the Services. This
								includes any data you enter in any “free text” box on our product, as well as files
								and links you upload to the Services. Examples of the content we collect and store
								include applications you create in the Services, descriptions of application-related
								commands, links to access applications, links to privacy policies for applications,
								or any other information you provide;
							</li>
							<li>
								if you purchase Acorn products or Services, financial or billing data, such as name,
								address, credit card number or bank account information;
							</li>
							<li>
								if you attend an Acorn event (in-person or online) or our offices, your attendance
								details (e.g., entry time);
							</li>
							<li>
								if you avail yourself of Acorn Services or submit a technical support case, your
								activity with Acorn to interact with and take part in these services;
							</li>
							<li>
								your image and/or voice, such as for phone/video call recordings (for training,
								quality assurance and administration purposes), if you attend an Acorn event (e.g.,
								CCTV), post an image/recording to an Acorn Site (e.g., add a profile photo to a
								review or profile) or you otherwise send a recording/image to Acorn;
							</li>
							<li>
								information from otherwise engaging/interacting with you, such as your feedback to
								us through our Services and responses to any promotional or survey communications;
								and/or
							</li>
						</ul>

						<p>
							We may also collect Personal Data about you from third parties, such as vendors (e.g.,
							web analytics tools, data enrichment providers) and Acorn partners (e.g., where your
							contact details are provided to enable delivery of Acorn software, for customer
							success purposes and/or in relation to maintenance/support, including renewal and/or
							cancellation of same). We also maintain social media pages and may collect Personal
							Data from you when you interact with these or communicate with us through our social
							media pages.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[3].id}>USE OF PERSONAL DATA</h2>
						<p>
							Depending on your relationship with Acorn, we may use your Personal Data for different
							purposes. Where required by applicable law in relation to the particular use, we will
							obtain your consent to collect and use your Personal Data. Otherwise, we will rely on
							another legal basis, such as in connection with a contract or for our legitimate
							interests, as set out below.
						</p>
						<ul class="list-disc pl-4">
							<li>
								<u>Perform a contract</u>: We may process your Personal Data in connection with a
								contract with you or your organization, such as to process relevant payments,
								deliver (customer) or receive (vendor) products/services.
							</li>
							<li>
								<u>Operate our Services</u>: We may use your Personal Data to provide our Services,
								content, and offerings (e.g., downloads, registrations for accounts or profiles,
								such as an Acorn Account), including necessary functionality, content customization
								(e.g., based on what we think will interest you), and functionality to enhance ease
								of use (e.g., make the Services easier for you to use by not making you enter
								information more than once, etc.). Unless such processing is in connection with a
								contract or our legitimate interests, it will be on the basis of your consent.
							</li>
							<li>
								<u>Improve our Services</u>: We process your interactions with our Services, content
								and offerings (e.g., Acorn support) to assess and improve these and other user
								experiences, such as by use of cookies (see section 6 below). This data is typically
								aggregated unless Personal Data is required for the particular processing operation.
								We process your Personal Data for this purpose for our legitimate interests of
								maintaining and improving our Services and providing tailored content, or where
								required by law, on the basis of your consent.
							</li>
							<li>
								<u>Manage contact requests</u>: We may use your Personal Data to reply to contact
								initiated by you, such as through a webform, chatbot, survey responses or Acorn
								support. We process your Personal Data for these purpose for our legitimate interest
								in replying to/fulfilling your request, collecting survey responses and to carry out
								our contractual obligations under the applicable terms (e.g., providing Acorn
								support).
							</li>
							<li>
								<u>Other communications</u>: We may use your Personal Data to otherwise communicate
								with you to manage our relationship with you/your organization, such as to manage
								our offerings, for customer success purposes, to contact you regarding updates to
								our offerings you may be interested in, and to run surveys. Such processing will be
								to fulfil our contractual obligations, where necessary for our legitimate interests
								or, where legally required, with your consent.
							</li>
							<li>
								<u>Billing</u>: We may use your personal data for billing, collection, and
								protection of our property and legal rights.
							</li>
							<li>
								<u>Call recording</u>: We may record phone or video calls (e.g., Zoom) for training,
								quality assurance, the functionality of programs, functionality, or features you
								have selected, and/or administration purposes. We process this Personal Data on the
								basis of our legitimate interests however, if required under applicable law, we will
								obtain your consent or give you the option to object to the call being recorded.
							</li>
							<li>
								<u>Marketing</u>: We may use your Personal Data to contact you for marketing
								purposes, such as to alert you to product upgrades, special offers, updated
								information/services from Acorn, and to contact you (e.g., by phone or email)
								regarding your interest in our offerings and/or invite you to Acorn events. Such
								contact will be on the basis of legitimate interest or, where legally required, on
								the basis of your consent. To opt-out of marketing contact, please unsubscribe here
								support@Acorn.io.
							</li>
							<li>
								<u>Events</u>: We may process your Personal Data to operate Acorn events, including
								training/education related activities, in-person and online, that you have selected
								to attend. Such processing will be on the basis of our legitimate interests or to
								perform a contract with you under the applicable terms of service.
							</li>
							<li>
								<u>Complicance and Security</u>: We process your Personal Data to comply with our
								legal obligations and to ensure compliant use/security, such as to ensure the use of
								our Services, content and offerings in compliance with our terms and/or to ensure
								their security, to cooperate with courts, regulators or other government authorities
								to the extent processing/disclosure of Personal Data is required under applicable
								law, where it is necessary to protect our legal rights or those of others, or for
								other compliance purposes such as auditing, investigations and responding to legal
								processes or lawful requests. Where we process health related data for Covid-19
								related reasons or to accommodate a reasonable adjustment for a disability, we will
								only do so to carry out our legal obligations relating to health and safety, under
								applicable law.
							</li>
							<li>
								<u>Professional advisers</u>: In individual instances, we may share your Personal
								Data with professional advisers acting as service providers, processors, or joint
								controllers - including lawyers, bankers, auditors, and insurers who provide
								consultancy, banking, legal, insurance and accounting services, and to the extent we
								are legally obliged to share or have a legitimate interest in sharing your Personal
								Data.
							</li>
							<li>
								<u>Corporate Transactions</u>: Third parties involved in a corporate transaction: If
								we are involved in a merger, reorganization, dissolution or other fundamental
								corporate change, or sell a website or business unit, or if all or a portion of our
								business, assets or stock are acquired by a third party. In accordance with
								applicable laws, we will use reasonable efforts to notify you of any transfer of
								Personal Data to an unaffiliated third party.
							</li>
							<li>
								<u>Combining Personal Data</u>: We may also combine Personal Data we hold about you.
								For example, we may combine your contact information with your usage data relating
								to our offerings and our Services for the purposes above, such as to improve our
								Services and our offerings, as well as to create more tailored experiences by
								providing content that may be of interest to you. We may also use your Personal Data
								to fulfil any other purposes disclosed to you at the time of collection or as
								otherwise required/permitted by applicable law.
							</li>
						</ul>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[4].id}>SHARING PERSONAL DATA</h2>
						<p>Acorn may share Personal Data for various purposes, such as:</p>
						<ul class="list-disc pl-4">
							<li>
								within the group of Acorn companies to operate our business and provide our Sites,
								products and Services;
							</li>
							<li>
								with authorized Acorn vendors, advisors and contractors providing services on our
								behalf (such as a technology or professional service providers) to operate our
								business, Sites, products and Services;
							</li>
							<li>
								if you register for or attend an Acorn event, Acorn may share participant
								information (e.g., your name, organization, work email) with other participants,
								organizers or hosts of the same event in order to facilitate the event and the
								subsequent exchange of ideas. Acorn may also share your contact details with event
								sponsors where you have consented, e.g., through registration, to these sponsors
								contacting you regarding their own offerings. Your contact details may also be used
								by other hosts of the same event, such as to send marketing material, according to
								their privacy notices provided to you;
							</li>
							<li>
								sharing with non-Acorn companies or entities where authorized or required by law.
								This can happen when we: (i) comply with court orders, subpoenas, and lawful
								discovery requests, and as otherwise authorized or required by law; (ii) detect and
								prevent fraud; (iii) provide or obtain information related to payment for your
								service, (iv) route your calls or other communications, (v) defend and enforce our
								legal rights.
							</li>
							<li>
								with automotive Advisors and their employers who may market or sell products to you
								(e.g., auto dealerships).
							</li>
						</ul>
						<p>
							<u>Artificial Intelligence Disclosure</u>: If the Services utilize APIs to access user
							data from third-party sources (e.g., Google Workspace), we will not use or share that
							third-party-sourced data for any artificial intelligence model training or evaluation
							purposes. We will only use data from such APIs for the purpose of providing and
							improving the Services. Additionally, your personal information will never be
							transferred, delivered, or made available to or otherwise used to train a generative
							AI product.
						</p>
						<p>
							Acorn will only share Personal Data to the extent needed to perform the relevant use
							and will take such steps as are necessary to safeguard Personal Data. For example, our
							vendors, advisors and contractors are required to keep such Personal Data confidential
							and not to use it other than for the purposes intended. Acorn partners are obliged to
							comply with the privacy rules set out in their agreement with Acorn. Acorn does not
							sell or rent Personal Data to third parties.
						</p>
						<p>
							Acorn may also disclose Personal Data to comply with legal requirements, such as in
							relation to legal proceedings or investigations by governmental or law enforcement
							agencies (including national security agencies), or to meet tax or other reporting
							requirements, including to (a) protect and defend the rights or property of Acorn,
							including the defense and management of legal claims and investigations, (b) act in
							urgent circumstances to protect the personal safety of users our Services, our Acorn
							team members, or the public, (c) as part of a merger or change in corporate ownership
							or control, and/or (d) as otherwise permitted or required by applicable law.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[5].id}>
							LEGAL BASIS FOR PROCESSING
						</h2>
						<p>
							If the legal basis for the processing is your consent, you have the right to withdraw
							at any time your consent to the processing, without this affecting the lawfulness of
							previous processing under your consent, until the date of your withdrawal. This might
							lead, however, to your inability to interact with the Services.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[6].id}>
							COOKIES AND SIMILAR TECHNOLOGIES
						</h2>
						<p>
							As is true of most websites, when you visit our Sites, cookies will be placed on your
							device (computer, mobile or tablet). A "cookie" is a small text file that is placed on
							your device by websites you visit. Some of the cookies we use are strictly necessary
							to operate our Sites. Others relate to the Site's performance, functionality and/or to
							advertising. Information relating to a cookie or similar technology may include
							identifiers such as IP address, and information like general location, browser type
							and language, and internet activity such as timestamps. Acorn uses the following types
							of cookies and similar technologies on our Sites:
						</p>
						<p>
							<u>Strictly Necessary</u>: These cookies enable the Sites to function correctly and
							deliver the Services and products you have requested. These cookies do not gather
							information about you that could be used for marketing or remembering other websites
							you have visited on the internet.
						</p>
						<p>
							<u>Functional</u>: These cookies do things like remember your preferred language,
							understand your preferences and associate users to forms submitted to enable
							pre-completion of subsequent forms as well as improve and customize your experience on
							our Sites.
						</p>
						<p>
							<u>Performance</u>: We use third-party analytics tools to help us analyze how our
							Sites and other electronic mediums are used, such as allowing us to compile reports on
							website activity, providing us other services relating to website activity and
							internet usage and whether email communications are opened or left unread.
						</p>
						<p>
							<u>Advertising</u>: These cookies may be set on our Sites by our advertising partners.
							They may be used to build a profile of your interests and/or show you relevant adverts
							on other sites. If you wish to not have your online information used for this purpose
							you can also visit resources such as https://optout.networkadvertising.org and/or
							https://www.youronlinechoices.eu (please note these and similar resources do not
							prevent you from being served ads as you will continue to receive generic ads).
						</p>
						<p>
							<u>Cookies can be session-based</u> (which disappear once you close your device or browser)
							or persistent (which remain on your device afterwards). Acorn may also rely on cookies
							or similar technology operating on other websites, for example to display our adverts to
							you. You can generally disable the use of cookies by changing your browser settings. You
							may also adjust your browser settings, however if you choose to not have your browser accept
							cookies from the Acorn Sites, you will not be able to experience a personalized visit and
							it may limit your ability to use some features on our Sites. For more information about
							cookies, visit https://www.aboutcookies.org. We may also use pixels, web beacons and similar
							technologies on our Sites and in emails, for example in a marketing email that notifies
							us if you click on a link in the email.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[7].id}>
							PROTECTIONS OF PERSONAL DATA
						</h2>
						<p>
							Acorn will retain and process Personal Data for a period of time consistent with the
							purpose of collection (see section 3 above) and/or as long as necessary to fulfil our
							legal obligations. We determine the applicable retention period by taking into account
							the (i) amount, nature and sensitivity of the Personal Data, (i) relevant use,
							including whether we can achieve the use through other means (e.g., by instead using
							deidentified data); (iii) potential risk of harm from unauthorized use or disclosure
							of the Personal Data, and (iv) applicable legal requirements (e.g., statutes of
							limitation).
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[8].id}>
							HOW LONG WE RETAIN PERSONAL DATA
						</h2>
						<p>
							Acorn will retain and process Personal Data for a period of time consistent with the
							purpose of collection (see section 3 above) and/or as long as necessary to fulfil our
							legal obligations. We determine the applicable retention period by taking into account
							the (i) amount, nature and sensitivity of the Personal Data, (i) relevant use,
							including whether we can achieve the use through other means (e.g., by instead using
							deidentified data); (iii) potential risk of harm from unauthorized use or disclosure
							of the Personal Data, and (iv) applicable legal requirements (e.g., statutes of
							limitation).
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[9].id}>CHILDREN’S PRIVACY</h2>
						<p>
							We don’t knowingly collect personal information from anyone under 18. We also won’t
							contact a child under 18 for marketing purposes without parental consent. Acorn
							Services are intended for adult use and not for use by minors.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[10].id}>
							YOUR RIGHTS AND CONTROLLING YOUR PERSONAL INFORMATION
						</h2>
						<p>
							If you are an individual in the European Union, the United Kingdom, or Switzerland,
							you have the following rights, pursuant to the privacy law applicable to Acorn:
						</p>
						<ul class="list-disc pl-4">
							<li>
								to request access to your personal data and request means to ensure that your
								personal data is correct and up-to-date;
							</li>
							<li>to request rectification of your personal data;</li>
							<li>to request erasure of your personal data;</li>
							<li>
								If you have an account with obot.ai, you can correct, update or delete your personal
								data in your obot.ai account. If you want to exercise your right to have your
								personal data erased from obot.ai records, please submit a request support@acorn.io;
							</li>
							<li>
								to request data portability to another data controller, if you have provided your
								personal data in a structured, commonly used and machine-readable format and this
								has been processed by Acorn based on your consent or based on a contract executed
								with you;
							</li>
							<li>
								to object to the processing of your personal data (including objection to profiling)
								if Acorn processes your personal data based on legitimate interest;
							</li>
							<li>the right to lodge a complaint with a supervisory authority.</li>
						</ul>

						<p>
							If you have declared your consent regarding certain collecting, processing and use of
							your personal data, you can revoke this consent at any time with future effect.
							Furthermore, you can object to the use of your personal data for marketing purposes
							without incurring any costs other than the transmission costs in accordance with the
							basic tariffs and without this affecting the use of the Services you have contracted.
							For example, if you have given your consent to Acorn in this respect, you may opt out
							of receiving marketing communications from us by using the unsubscribe link within
							each email, or by contacting us and requesting that you are removed from our marketing
							email list or registration database.
						</p>
						<p>
							Also, if you are an individual outside the European Union, the United Kingdom, or
							Switzerland, Acorn provides you with means to ensure that your personal data is
							correct and up-to-date. You can access your personal data by contacting Acorn. You can
							also update your personal data in your obot.ai account. You are entitled to request
							the correction, update, restriction and deletion of your personal data under the
							conditions mentioned in this Privacy Policy. You have the right to object at any time,
							based on legitimate grounds, regarding the processing of your personal data by Acorn.
						</p>
						<p>
							Non-discrimination: We will not discriminate against you for exercising any of your
							rights over your personal information. Unless your personal information is required to
							provide you with a particular service or offer (for example providing user support),
							we will not deny you goods or services and/or charge you different prices or rates for
							goods or services, including through granting discounts or other benefits, or imposing
							penalties, or provide you with a different level or quality of goods or services.
						</p>
						<p>
							Notification of data breaches: We will comply with laws applicable to us in respect of
							any data breach.
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[11].id}>
							CONTACT OR COMPLAINT INFORMATION
						</h2>
						<p>
							Acorn strives to resolve complaints about your privacy and our collection or use of
							your personal information. If you have a complaint please write to support@acorn.io
						</p>
					</li>
					<li>
						<h2 class="text-lg font-semibold" id={sectionHeaders[12].id}>CHANGES TO THIS NOTICE</h2>
						<p>
							Acorn reserves the right to modify or update this Notice from time to time to reflect
							changes in technology, our practices, law and other factors impacting the
							collection/use of Personal Data. You are encouraged to regularly check this Notice for
							any updates. We may also collect, use and disclose Personal Data for other purposes
							otherwise disclosed to you at the time of collection/processing in a supplementary
							notice.
						</p>
						<p>Last updated: April 22, 2025</p>
					</li>
				</ol>
			</div>
		</div>
	</main>
	<Footer />
</div>

<style lang="postcss">
	.doc ol {
		list-style-type: none;
		counter-reset: item;
		margin: 0;
		padding: 0 0 1rem 0;
	}

	.doc ol > li {
		display: table;
		counter-increment: item;
		margin-bottom: 1rem;
		margin-top: 1rem;
	}

	.doc ol > li:before {
		content: counters(item, '.') '. ';
		display: table-cell;
		padding-right: 1rem;
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

	.doc li p {
		padding-bottom: 1rem;
	}

	.doc ul li {
		padding-bottom: 1rem;
	}
</style>
